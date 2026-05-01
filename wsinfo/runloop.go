package wsinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/synthetixio/synthetix-go/types"
)

// runLoop owns connection lifecycle. It dials, on success spawns the
// read loop and (optionally) the ping loop, replays all active
// subscriptions on the new conn, and blocks until the read loop
// exits. On exit (for any reason other than client shutdown) it
// backs off and reconnects.
//
// Invariant: the Client is usable from the moment NewClient returns;
// Subscribe can be called before the first connect. If the conn is
// nil at Subscribe time, sendRequest returns nil and the subscribe
// is replayed on the next successful dial.
func (c *Client) runLoop() {
	defer close(c.runDone)

	backoff := c.cfg.ReconnectInitial
	for {
		if c.runCtx.Err() != nil {
			return
		}

		conn, err := c.dial()
		if err != nil {
			if c.logger != nil {
				c.logger.Warn("wsinfo: dial failed, will retry",
					"url", c.wsURL, "error", err, "backoff", backoff)
			}
			if !c.sleepWithJitter(backoff) {
				return
			}
			backoff = nextBackoff(backoff, c.cfg.ReconnectMax)
			continue
		}

		// Connected. Reset backoff, bump connGen so prior wire-sub
		// state is invalidated, store the conn so Subscribe can
		// write to it.
		backoff = c.cfg.ReconnectInitial
		c.mu.Lock()
		c.conn = conn
		c.connGen++
		c.mu.Unlock()

		if c.logger != nil {
			c.logger.Info("wsinfo: connected", "url", c.wsURL)
		}

		// Start read (and optionally ping) loops BEFORE replaying
		// subscriptions, so replies to the replay can be routed
		// straight back to the waiting sendRequest callers instead
		// of sitting unread in the TCP buffer.
		readDone := make(chan struct{})
		go func() {
			c.readLoop(conn)
			close(readDone)
		}()

		pingDone := make(chan struct{})
		if c.cfg.PingInterval > 0 {
			go c.pingLoop(conn, pingDone)
		} else {
			close(pingDone)
		}

		c.resubscribeAll()

		<-readDone
		if c.cfg.PingInterval > 0 {
			close(pingDone)
		}
		_ = conn.Close()

		// Conn dead. Fail any pending replies so callers unblock.
		c.failPendingReplies()
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()

		if c.runCtx.Err() != nil {
			return
		}
		if c.logger != nil {
			c.logger.Warn("wsinfo: disconnected, will reconnect", "backoff", backoff)
		}
		if !c.sleepWithJitter(backoff) {
			return
		}
		backoff = nextBackoff(backoff, c.cfg.ReconnectMax)
	}
}

// dial establishes the upstream connection.
func (c *Client) dial() (*websocket.Conn, error) {
	hdr := http.Header{}
	hdr.Set("User-Agent", c.cfg.UserAgent)

	dialCtx, cancel := context.WithTimeout(c.runCtx, c.cfg.DialTimeout)
	defer cancel()

	conn, _, err := c.dialer.DialContext(dialCtx, c.wsURL, hdr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", c.wsURL, err)
	}
	return conn, nil
}

// resubscribeAll re-issues subscribe requests for every key currently
// tracked, using the gated sendSubscribeIfNeeded path so we don't
// race-duplicate subscribes with a live local Subscribe caller.
func (c *Client) resubscribeAll() {
	c.mu.Lock()
	keys := make([]string, 0, len(c.subscriptions))
	for k := range c.subscriptions {
		keys = append(keys, k)
	}
	c.mu.Unlock()

	for _, key := range keys {
		ctx, cancel := context.WithTimeout(c.runCtx, c.cfg.SubscribeTimeout)
		if err := c.sendSubscribeIfNeeded(ctx, key); err != nil && c.logger != nil {
			c.logger.Warn("wsinfo: resubscribe failed",
				"key", key, "error", err)
		}
		cancel()
	}
}

// readLoop reads frames off the conn and demuxes them to pending
// reply channels or to fan-out subscriptions. Returns on any read
// error (which tears down the connection).
func (c *Client) readLoop(conn *websocket.Conn) {
	for {
		if err := conn.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout)); err != nil {
			if c.logger != nil {
				c.logger.Warn("wsinfo: SetReadDeadline failed", "error", err)
			}
			return
		}
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if c.logger != nil && !isExpectedCloseError(err) {
				c.logger.Warn("wsinfo: read error", "error", err)
			}
			return
		}

		msg := &types.WSMessage{}
		if decodeErr := json.Unmarshal(raw, msg); decodeErr != nil {
			if c.logger != nil {
				c.logger.Warn("wsinfo: decode message failed",
					"error", decodeErr, "bytes", len(raw))
			}
			continue
		}
		msg.Raw = append(json.RawMessage(nil), raw...)

		// Demux: reply vs notification.
		if msg.ID != "" && !msg.IsNotification() {
			c.deliverReply(msg)
			continue
		}
		c.fanOut(msg)
	}
}

// deliverReply routes a request-response reply to the waiting
// sendRequest caller, if any.
func (c *Client) deliverReply(msg *types.WSMessage) {
	c.mu.Lock()
	ch, ok := c.pendingReplies[msg.ID]
	if ok {
		delete(c.pendingReplies, msg.ID)
	}
	c.mu.Unlock()
	if ok {
		select {
		case ch <- msg:
		default:
		}
	}
}

// failPendingReplies unblocks every in-flight sendRequest with a nil
// reply so they return a TransportError.
func (c *Client) failPendingReplies() {
	c.mu.Lock()
	pending := c.pendingReplies
	c.pendingReplies = make(map[string]chan *types.WSMessage)
	c.mu.Unlock()
	for _, ch := range pending {
		select {
		case ch <- nil:
		default:
		}
	}
}

// fanOut dispatches a notification to every local subscriber whose
// spec matches the message's channel and symbol.
//
// Matching policy:
//   - Every message is delivered to every subscription whose
//     notificationChannel() matches msg.Channel AND whose spec.Symbol
//     matches the data's symbol (when both are present).
//   - If msg.Channel is empty we match by msg.Method instead
//     (defensive fallback).
//   - If the subscriber's spec has no usable channel mapping (future
//     channels) we forward based on Type-string equality.
//
// We don't reparse data for every match; we peek the symbol once and
// reuse it across all subscriptions in this dispatch.
func (c *Client) fanOut(msg *types.WSMessage) {
	channel := msg.Channel
	if channel == "" {
		channel = msg.Method
	}
	symbol := extractSymbol(msg.Data)

	c.mu.Lock()
	var matched []*subscriber
	for _, s := range c.subscriptions {
		if !subscriptionMatches(s.spec, channel, symbol) {
			continue
		}
		matched = append(matched, s.subs...)
	}
	c.mu.Unlock()

	for _, sub := range matched {
		sub.enqueue(msg)
	}
}

// subscriptionMatches reports whether a spec should receive a
// notification with the given channel name and symbol.
func subscriptionMatches(spec SubscribeSpec, channel, symbol string) bool {
	wantChannel := spec.notificationChannel()
	if wantChannel == "" {
		wantChannel = spec.Type
	}
	if channel != wantChannel {
		return false
	}
	if symbol == "" {
		// Unidentifiable messages fan out to every matching-channel
		// subscription; subscribers with strict symbol requirements
		// should verify in their handler.
		return true
	}
	return equalFoldTrim(symbol, spec.Symbol)
}

// extractSymbol pulls the "symbol" or "market" field out of a data
// payload without fully decoding it into a typed struct. Returns ""
// on any decode failure or missing field.
func extractSymbol(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}
	var peek struct {
		Symbol string `json:"symbol"`
		Market string `json:"market"`
	}
	if err := json.Unmarshal(data, &peek); err != nil {
		return ""
	}
	if peek.Symbol != "" {
		return peek.Symbol
	}
	return peek.Market
}

func equalFoldTrim(a, b string) bool {
	a = trimSpaceAndUpper(a)
	b = trimSpaceAndUpper(b)
	return a == b
}

func trimSpaceAndUpper(s string) string {
	// Manual inlined equivalent of strings.ToUpper(strings.TrimSpace(s))
	// kept local here to avoid an extra import in the hot path. Small
	// enough that the allocator is happy.
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	if start == 0 && end == len(s) {
		return upperASCII(s)
	}
	return upperASCII(s[start:end])
}

func upperASCII(s string) string {
	hasLower := false
	for i := 0; i < len(s); i++ {
		if s[i] >= 'a' && s[i] <= 'z' {
			hasLower = true
			break
		}
	}
	if !hasLower {
		return s
	}
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

// pingLoop sends an application-level ping every PingInterval. Exits
// when done is closed.
func (c *Client) pingLoop(conn *websocket.Conn, done <-chan struct{}) {
	ticker := time.NewTicker(c.cfg.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-c.runCtx.Done():
			return
		case <-ticker.C:
			_ = c.writeJSON(conn, types.WSRequest{
				ID:     c.allocRequestID(),
				Method: "ping",
			})
		}
	}
}

// sleepWithJitter sleeps for d ± 25% jitter. Returns false if the
// client was closed during the sleep.
func (c *Client) sleepWithJitter(d time.Duration) bool {
	if d <= 0 {
		return c.runCtx.Err() == nil
	}
	jitter := time.Duration(rand.Int63n(int64(d) / 2))
	dur := d - d/4 + jitter
	select {
	case <-c.runCtx.Done():
		return false
	case <-time.After(dur):
		return true
	}
}

// nextBackoff doubles d, capped at maxBackoff.
func nextBackoff(d, maxBackoff time.Duration) time.Duration {
	next := d * 2
	if next > maxBackoff {
		return maxBackoff
	}
	return next
}

// isExpectedCloseError filters the noisy "normal close" conditions
// out of the reconnect log line.
func isExpectedCloseError(err error) bool {
	return websocket.IsCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
	)
}

// ---------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------

// TransportError is returned for network-layer failures on a
// subscribe/unsubscribe/ping. Op identifies the method that failed.
type TransportError struct {
	Op  string
	Err error
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("wsinfo: transport error on %s: %v", e.Op, e.Err)
}

func (e *TransportError) Unwrap() error { return e.Err }

// WSReplyError is returned when the server replies with a structured
// error envelope (status >= 400 or a non-nil error object).
type WSReplyError struct {
	Op      string
	Status  int
	Code    int
	Message string
}

func (e *WSReplyError) Error() string {
	return fmt.Sprintf("wsinfo: %s returned status=%d code=%d: %s",
		e.Op, e.Status, e.Code, e.Message)
}
