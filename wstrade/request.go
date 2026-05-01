package wstrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/synthetixio/synthetix-go/types"
)

func (c *Client) sendRequest(ctx context.Context, method string, params map[string]any) (*types.WSMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.cfg.RequestTimeout)
		defer cancel()
	}

	reqID := fmt.Sprintf("%d", c.nextID())
	replyCh := make(chan *types.WSMessage, 1)

	c.mu.Lock()
	conn, err := c.waitForConnLocked(ctx, method == "auth")
	if err != nil {
		c.mu.Unlock()
		return nil, err
	}
	c.pendingReplies[reqID] = replyCh
	c.mu.Unlock()

	req := types.WSRequest{ID: reqID, Method: method, Params: params}
	body, err := json.Marshal(req)
	if err != nil {
		c.removePending(reqID)
		return nil, fmt.Errorf("wstrade: marshal %s request: %w", method, err)
	}

	c.writeMu.Lock()
	_ = conn.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout))
	err = conn.WriteMessage(1, body)
	c.writeMu.Unlock()
	if err != nil {
		c.removePending(reqID)
		return nil, fmt.Errorf("wstrade: write %s request: %w", method, err)
	}

	select {
	case <-ctx.Done():
		c.removePending(reqID)
		return nil, ctx.Err()
	case msg := <-replyCh:
		if msg == nil {
			return nil, errors.New("wstrade: connection closed while waiting for reply")
		}
		return msg, nil
	}
}

func (c *Client) sendRequestFireAndForget(method string, params map[string]any) error {
	req := types.WSRequest{ID: fmt.Sprintf("%d", c.nextID()), Method: method, Params: params}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return nil
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout))
	return conn.WriteMessage(1, body)
}

func (c *Client) nextID() uint64 {
	return atomic.AddUint64(&c.nextReqID, 1)
}

func (c *Client) waitForConnLocked(ctx context.Context, allowUnauthenticated bool) (*websocket.Conn, error) {
	if c.closed {
		return nil, errors.New("wstrade: client is closed")
	}
	if c.conn != nil && (allowUnauthenticated || c.authenticated) {
		return c.conn, nil
	}
	if c.connReady == nil {
		return nil, errors.New("wstrade: websocket is not connected")
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			c.mu.Lock()
			c.connReady.Broadcast()
			c.mu.Unlock()
		case <-done:
		}
	}()
	defer close(done)
	for (c.conn == nil || (!allowUnauthenticated && !c.authenticated)) && !c.closed && ctx.Err() == nil {
		c.connReady.Wait()
	}
	if c.closed {
		return nil, errors.New("wstrade: client is closed")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if c.conn == nil || (!allowUnauthenticated && !c.authenticated) {
		return nil, errors.New("wstrade: websocket is not connected")
	}
	return c.conn, nil
}

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

func (c *Client) removePending(id string) {
	c.mu.Lock()
	delete(c.pendingReplies, id)
	c.mu.Unlock()
}

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

func (c *Client) fanOut(msg *types.WSMessage) {
	c.mu.Lock()
	handlers := make([]Handler, 0, len(c.subscribers))
	for _, h := range c.subscribers {
		handlers = append(handlers, h)
	}
	c.mu.Unlock()
	for _, h := range handlers {
		func(handler Handler) {
			defer func() {
				if recover() != nil && c.logger != nil {
					c.logger.Warn("wstrade: subscriber handler panicked")
				}
			}()
			handler(msg)
		}(h)
	}
}

// WSError is returned when /v1/ws/trade replies with a structured
// websocket error.
type WSError struct {
	Method  string
	Code    int
	Message string
}

func (e *WSError) Error() string {
	return fmt.Sprintf("wstrade: %s returned websocket error %d: %s", e.Method, e.Code, e.Message)
}

