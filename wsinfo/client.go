package wsinfo

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	sdklogger "github.com/synthetixio/synthetix-go/logger"
	"github.com/synthetixio/synthetix-go/types"
)

// Defaults applied when Config fields are zero.
const (
	DefaultDialTimeout       = 10 * time.Second
	DefaultWriteTimeout      = 5 * time.Second
	DefaultReadTimeout       = 90 * time.Second
	DefaultReconnectInitial  = 500 * time.Millisecond
	DefaultReconnectMax      = 30 * time.Second
	DefaultSubscribeTimeout  = 10 * time.Second
	DefaultPingInterval      = 30 * time.Second
	DefaultUserAgent         = "synthetix-go/wsinfo"
)

// Config configures a wsinfo Client.
type Config struct {
	// BaseURL is the Synthetix API root (http/https). wsinfo converts
	// http->ws and https->wss and appends /v1/ws/info. Required.
	BaseURL string

	// DialTimeout bounds the initial handshake. Defaults to
	// DefaultDialTimeout when zero.
	DialTimeout time.Duration

	// WriteTimeout is the deadline applied to each websocket write
	// (subscribe/unsubscribe/ping). Defaults to DefaultWriteTimeout.
	WriteTimeout time.Duration

	// ReadTimeout is the deadline refreshed on each inbound frame
	// (including pongs). A missed deadline triggers reconnect.
	// Defaults to DefaultReadTimeout.
	ReadTimeout time.Duration

	// ReconnectInitial is the backoff used on the first reconnect
	// attempt. Each subsequent attempt doubles, capped at
	// ReconnectMax. Defaults to DefaultReconnectInitial.
	ReconnectInitial time.Duration

	// ReconnectMax caps the backoff between reconnect attempts.
	// Defaults to DefaultReconnectMax.
	ReconnectMax time.Duration

	// SubscribeTimeout bounds how long Subscribe waits for the
	// upstream ack. Defaults to DefaultSubscribeTimeout.
	SubscribeTimeout time.Duration

	// PingInterval is how often wsinfo sends a client-side
	// application-level ping to keep the connection warm. Defaults
	// to DefaultPingInterval. Zero disables client pings entirely
	// (reliance on server pings only).
	PingInterval time.Duration

	// UserAgent overrides DefaultUserAgent on the initial handshake.
	UserAgent string

	// SubscriberBufferSize is the per-subscriber ring-buffer depth.
	// Zero uses defaultSubscriberBufferSize (256).
	SubscriberBufferSize int

	// Dialer lets tests inject an *websocket.Dialer backed by an
	// httptest server. If nil, websocket.DefaultDialer is used with
	// DialTimeout applied.
	Dialer *websocket.Dialer

	// Logger is the structured logger. Nil is allowed.
	Logger sdklogger.Logger
}

// Client is the shared /v1/ws/info upstream. Safe for concurrent
// use from multiple goroutines.
type Client struct {
	cfg      Config
	wsURL    string
	dialer   *websocket.Dialer
	logger   sdklogger.Logger

	mu            sync.Mutex
	conn          *websocket.Conn
	connGen       uint64                           // incremented on every successful dial; 0 == never connected
	subscriptions map[string]*subscription         // keyed by SubscribeSpec.key()
	pendingReplies map[string]chan *types.WSMessage // keyed by request id
	nextReqID      uint64
	nextSubID      uint64
	closed         bool

	// writeMu serialises writes on the active conn. gorilla/websocket
	// requires that only one goroutine writes at a time; we hold this
	// separately from mu so a long write can't block Subscribe's
	// housekeeping.
	writeMu sync.Mutex

	runCtx    context.Context
	runCancel context.CancelFunc
	runDone   chan struct{}
}

// subscription is a shared upstream subscription that may have
// multiple local subscribers. Guarded by Client.mu for all mutations
// of subs / spec / lastWireGen; subscribers themselves are
// independent.
//
// lastWireGen is the value of Client.connGen at which this
// subscription last had a wire-level subscribe issued. Used to
// deduplicate the race between a local Subscribe caller and the
// runLoop's resubscribeAll on the same conn: both race to set
// lastWireGen == current connGen, and whoever sets it first actually
// writes to the wire.
type subscription struct {
	spec        SubscribeSpec
	subs        []*subscriber
	lastWireGen uint64
}

// NewClient constructs and starts a wsinfo client. The returned
// client is running: Subscribe can be called immediately; the client
// connects lazily on the first Subscribe.
//
// Call Close() to tear down.
func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("wsinfo: BaseURL is required")
	}
	wsURL, err := toWSURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}

	applyDefaults(&cfg)

	dialer := cfg.Dialer
	if dialer == nil {
		dialer = &websocket.Dialer{
			HandshakeTimeout: cfg.DialTimeout,
		}
	}

	runCtx, runCancel := context.WithCancel(context.Background())
	c := &Client{
		cfg:            cfg,
		wsURL:          wsURL,
		dialer:         dialer,
		logger:         cfg.Logger,
		subscriptions:  make(map[string]*subscription),
		pendingReplies: make(map[string]chan *types.WSMessage),
		runCtx:         runCtx,
		runCancel:      runCancel,
		runDone:        make(chan struct{}),
	}
	go c.runLoop()
	return c, nil
}

func applyDefaults(c *Config) {
	if c.DialTimeout <= 0 {
		c.DialTimeout = DefaultDialTimeout
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = DefaultWriteTimeout
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = DefaultReadTimeout
	}
	if c.ReconnectInitial <= 0 {
		c.ReconnectInitial = DefaultReconnectInitial
	}
	if c.ReconnectMax <= 0 {
		c.ReconnectMax = DefaultReconnectMax
	}
	if c.SubscribeTimeout <= 0 {
		c.SubscribeTimeout = DefaultSubscribeTimeout
	}
	if c.PingInterval < 0 {
		c.PingInterval = 0
	} else if c.PingInterval == 0 {
		c.PingInterval = DefaultPingInterval
	}
	if c.UserAgent == "" {
		c.UserAgent = DefaultUserAgent
	}
}

// toWSURL converts an http(s)://host/... BaseURL to the fully-qualified
// ws(s)://host/v1/ws/info URL wsinfo connects to. Accepts BaseURL with
// or without trailing slash.
func toWSURL(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("wsinfo: invalid BaseURL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "ws":
		u.Scheme = "ws"
	case "https", "wss":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("wsinfo: unsupported BaseURL scheme %q", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/ws/info"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// Close tears down the client. In-flight subscribers receive no
// further notifications. Idempotent.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	for _, s := range c.subscriptions {
		for _, sub := range s.subs {
			sub.close()
		}
	}
	c.subscriptions = nil
	c.mu.Unlock()

	c.runCancel()
	<-c.runDone
	return nil
}

// Subscribe registers a local subscriber for the given spec. It
// returns an unsubscribe func that must be called to release the
// subscription, and an error if the spec is malformed or the
// upstream subscribe fails.
//
// Multiple Subscribes for the same spec (same key()) share one
// upstream subscription; wsinfo only issues the wire-level subscribe
// when the first local caller shows up.
func (c *Client) Subscribe(ctx context.Context, spec SubscribeSpec, h Handler) (func(), error) {
	if h == nil {
		return nil, errors.New("wsinfo: Handler is required")
	}
	if err := spec.validate(); err != nil {
		return nil, err
	}
	key := spec.key()

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("wsinfo: client closed")
	}
	existing, shared := c.subscriptions[key]
	if !shared {
		existing = &subscription{spec: spec}
		c.subscriptions[key] = existing
	}
	c.nextSubID++
	sub := newSubscriber(c.nextSubID, h, key, c.cfg.SubscriberBufferSize, c.logger)
	existing.subs = append(existing.subs, sub)
	firstLocalSub := !shared
	c.mu.Unlock()

	go sub.deliverLoop()

	// Wait for a live connection before attempting the wire-level
	// subscribe. This gives us observable error feedback from the
	// server (otherwise a first-connect subscribe would return nil
	// even if the server later replies with an error envelope).
	if firstLocalSub {
		waitCtx := ctx
		if deadline, ok := waitCtx.Deadline(); !ok || time.Until(deadline) > c.cfg.SubscribeTimeout {
			var cancel context.CancelFunc
			waitCtx, cancel = context.WithTimeout(waitCtx, c.cfg.SubscribeTimeout)
			defer cancel()
		}
		if c.waitForConn(waitCtx) == nil {
			// No conn before deadline; the runLoop will replay on
			// connect. Leave the subscription registered and return
			// success: the caller's handler will start receiving
			// events once upstream is reachable.
			return func() { c.removeSubscriber(key, sub) }, nil
		}
		if err := c.sendSubscribeIfNeeded(ctx, key); err != nil {
			c.removeSubscriber(key, sub)
			return nil, err
		}
	}

	return func() { c.removeSubscriber(key, sub) }, nil
}

// removeSubscriber removes one subscriber and, if it was the last
// subscriber for the key, sends an upstream unsubscribe.
func (c *Client) removeSubscriber(key string, sub *subscriber) {
	sub.close()
	c.mu.Lock()
	s, ok := c.subscriptions[key]
	if !ok {
		c.mu.Unlock()
		return
	}
	for i, existing := range s.subs {
		if existing == sub {
			s.subs = append(s.subs[:i], s.subs[i+1:]...)
			break
		}
	}
	lastSub := len(s.subs) == 0
	spec := s.spec
	if lastSub {
		delete(c.subscriptions, key)
	}
	c.mu.Unlock()

	if lastSub {
		if err := c.sendUnsubscribe(context.Background(), spec); err != nil && c.logger != nil {
			c.logger.Warn("wsinfo: unsubscribe failed (will be implicit on reconnect)",
				"key", key, "error", err)
		}
	}
}

// sendSubscribeIfNeeded sends a wire-level subscribe for the given
// key only if no subscribe has already been issued for the current
// connection generation. Safe to call from multiple goroutines (the
// runLoop's resubscribeAll + the local Subscribe caller race here);
// exactly one call per (key, connGen) tuple actually writes.
//
// Returns nil if there's no connection yet (runLoop will call us
// again after the next successful dial).
func (c *Client) sendSubscribeIfNeeded(ctx context.Context, key string) error {
	c.mu.Lock()
	s, ok := c.subscriptions[key]
	if !ok {
		c.mu.Unlock()
		return nil
	}
	gen := c.connGen
	if gen == 0 {
		// Not yet connected. Resubscribe-on-connect will handle it.
		c.mu.Unlock()
		return nil
	}
	if s.lastWireGen == gen {
		c.mu.Unlock()
		return nil
	}
	s.lastWireGen = gen
	spec := s.spec
	c.mu.Unlock()

	if err := c.sendRequest(ctx, "subscribe", spec.toWireParams()); err != nil {
		// Reset the wire-gen so a retry is possible on next reconcile.
		c.mu.Lock()
		if cur, ok := c.subscriptions[key]; ok && cur.lastWireGen == gen {
			cur.lastWireGen = 0
		}
		c.mu.Unlock()
		return err
	}
	return nil
}

// sendUnsubscribe writes an unsubscribe request; does not require ack.
func (c *Client) sendUnsubscribe(ctx context.Context, spec SubscribeSpec) error {
	return c.sendRequestFireAndForget(ctx, "unsubscribe", spec.toWireParams())
}

// sendRequest writes a request envelope with a fresh id, waits for
// the matching reply or for subscribeTimeout. Returns non-nil on
// transport failure, request timeout, or a structured WSError reply.
//
// If no connection is currently established, sendRequest waits up to
// SubscribeTimeout for one. If the client is still disconnected at
// deadline, it returns nil (best-effort: the runLoop will replay the
// subscription on the next successful dial).
func (c *Client) sendRequest(ctx context.Context, method string, params map[string]any) error {
	reqID := c.allocRequestID()
	replyCh := make(chan *types.WSMessage, 1)

	c.mu.Lock()
	c.pendingReplies[reqID] = replyCh
	conn := c.conn
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pendingReplies, reqID)
		c.mu.Unlock()
	}()

	waitCtx := ctx
	if deadline, ok := waitCtx.Deadline(); !ok || time.Until(deadline) > c.cfg.SubscribeTimeout {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(waitCtx, c.cfg.SubscribeTimeout)
		defer cancel()
	}

	if conn == nil {
		conn = c.waitForConn(waitCtx)
		if conn == nil {
			return nil
		}
	}

	if err := c.writeJSON(conn, types.WSRequest{ID: reqID, Method: method, Params: params}); err != nil {
		return &TransportError{Op: method, Err: err}
	}

	select {
	case <-waitCtx.Done():
		return fmt.Errorf("wsinfo: %s timed out: %w", method, waitCtx.Err())
	case reply := <-replyCh:
		if reply == nil {
			return &TransportError{Op: method, Err: errors.New("connection lost before reply")}
		}
		if reply.Error != nil || reply.Status >= 400 {
			code := 0
			msg := ""
			if reply.Error != nil {
				code = reply.Error.Code
				msg = reply.Error.Message
			}
			return &WSReplyError{Op: method, Status: reply.Status, Code: code, Message: msg}
		}
		return nil
	}
}

// waitForConn polls c.conn every 5ms until non-nil or ctx fires.
// Returns the conn on success, nil on ctx timeout.
func (c *Client) waitForConn(ctx context.Context) *websocket.Conn {
	t := time.NewTicker(5 * time.Millisecond)
	defer t.Stop()
	for {
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn != nil {
			return conn
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// sendRequestFireAndForget writes a request with no reply tracking.
func (c *Client) sendRequestFireAndForget(_ context.Context, method string, params map[string]any) error {
	reqID := c.allocRequestID()
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return nil
	}
	return c.writeJSON(conn, types.WSRequest{ID: reqID, Method: method, Params: params})
}

func (c *Client) allocRequestID() string {
	n := atomic.AddUint64(&c.nextReqID, 1)
	return fmt.Sprintf("wsinfo-%d", n)
}

// writeJSON writes a JSON-encoded message with the configured write
// deadline. Serialises writes under Client.mu-scoped conn ownership
// via a dedicated write mutex (conns are not safe for concurrent
// writes per gorilla/websocket docs).
func (c *Client) writeJSON(conn *websocket.Conn, v any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout))
	return conn.WriteJSON(v)
}
