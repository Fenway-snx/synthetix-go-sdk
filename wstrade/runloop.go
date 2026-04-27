package wstrade

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/synthetixio/synthetix-go/eip712"
	"github.com/synthetixio/synthetix-go/types"
)

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
				c.logger.Warn("wstrade: dial failed, will retry", "url", c.wsURL, "error", err, "backoff", backoff)
			}
			if !c.sleepWithJitter(backoff) {
				return
			}
			backoff = nextBackoff(backoff, c.cfg.ReconnectMax)
			continue
		}
		backoff = c.cfg.ReconnectInitial

		c.mu.Lock()
		c.conn = conn
		c.authenticated = false
		c.mu.Unlock()

		readDone := make(chan struct{})
		go func() {
			c.readLoop(conn)
			close(readDone)
		}()

		if err := c.authenticate(); err != nil {
			if c.logger != nil {
				c.logger.Warn("wstrade: auth failed", "error", err)
			}
			_ = conn.Close()
			<-readDone
			c.failPendingReplies()
			c.mu.Lock()
			c.conn = nil
			c.authenticated = false
			c.connReady.Broadcast()
			c.mu.Unlock()
			if !c.sleepWithJitter(backoff) {
				return
			}
			backoff = nextBackoff(backoff, c.cfg.ReconnectMax)
			continue
		}

		pingDone := make(chan struct{})
		if c.cfg.PingInterval > 0 {
			go c.pingLoop(conn, pingDone)
		} else {
			close(pingDone)
		}
		c.mu.Lock()
		c.authenticated = true
		c.connReady.Broadcast()
		c.mu.Unlock()

		<-readDone
		if c.cfg.PingInterval > 0 {
			close(pingDone)
		}
		_ = conn.Close()
		c.failPendingReplies()
		c.mu.Lock()
		c.conn = nil
		c.authenticated = false
		c.connReady.Broadcast()
		c.mu.Unlock()

		if c.runCtx.Err() != nil {
			return
		}
		if !c.sleepWithJitter(backoff) {
			return
		}
		backoff = nextBackoff(backoff, c.cfg.ReconnectMax)
	}
}

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

func (c *Client) authenticate() error {
	sig, timestamp, err := c.cfg.Signer.SignAuthMessage(c.cfg.SubAccountID, 0)
	if err != nil {
		return err
	}
	typedData := eip712.BuildAuthMessage(c.cfg.SubAccountID, timestamp, eip712.ActionWebSocketAuth)
	typedData.Message["subAccountId"] = fmt.Sprintf("0x%x", c.cfg.SubAccountID)
	typedData.Message["timestamp"] = fmt.Sprintf("0x%x", timestamp)
	message, err := json.Marshal(typedData)
	if err != nil {
		return fmt.Errorf("marshal auth message: %w", err)
	}
	ctx, cancel := context.WithTimeout(c.runCtx, c.cfg.RequestTimeout)
	defer cancel()
	msg, err := c.sendRequest(ctx, "auth", map[string]any{
		"message":   string(message),
		"signature": signatureHex(sig),
	})
	if err != nil {
		return err
	}
	if msg.Error != nil {
		return &WSError{Method: "auth", Code: msg.Error.Code, Message: msg.Error.Message}
	}
	return nil
}

func (c *Client) readLoop(conn *websocket.Conn) {
	for {
		if err := conn.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout)); err != nil {
			return
		}
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		msg := &types.WSMessage{}
		if err := json.Unmarshal(raw, msg); err != nil {
			if c.logger != nil {
				c.logger.Warn("wstrade: decode message failed", "error", err)
			}
			continue
		}
		msg.Raw = append(json.RawMessage(nil), raw...)
		if msg.ID == "" {
			msg.ID = msg.RequestID
		}
		if msg.ID != "" && !msg.IsNotification() {
			c.deliverReply(msg)
			continue
		}
		c.fanOut(msg)
	}
}

func signatureHex(sig types.SignatureComponents) string {
	r := strings.TrimPrefix(sig.R, "0x")
	s := strings.TrimPrefix(sig.S, "0x")
	return fmt.Sprintf("0x%s%s%02x", r, s, sig.V)
}

func (c *Client) pingLoop(conn *websocket.Conn, done <-chan struct{}) {
	ticker := time.NewTicker(c.cfg.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			_ = c.sendRequestFireAndForget("ping", nil)
		}
	}
}

func (c *Client) sleepWithJitter(d time.Duration) bool {
	if d <= 0 {
		d = c.cfg.ReconnectInitial
	}
	jitter := time.Duration(rand.Int63n(int64(d / 4)))
	timer := time.NewTimer(d + jitter)
	defer timer.Stop()
	select {
	case <-c.runCtx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func nextBackoff(cur, max time.Duration) time.Duration {
	if cur <= 0 {
		cur = DefaultReconnectInitial
	}
	next := cur * 2
	if next > max {
		return max
	}
	return next
}

