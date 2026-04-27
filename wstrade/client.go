// Package wstrade is an authenticated client for the Synthetix V4
// /v1/ws/trade WebSocket endpoint.
package wstrade

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	sdklogger "github.com/synthetixio/synthetix-go/logger"
	"github.com/synthetixio/synthetix-go/signer"
	"github.com/synthetixio/synthetix-go/types"
)

const (
	DefaultDialTimeout      = 10 * time.Second
	DefaultWriteTimeout     = 5 * time.Second
	DefaultReadTimeout      = 90 * time.Second
	DefaultReconnectInitial = 500 * time.Millisecond
	DefaultReconnectMax     = 30 * time.Second
	DefaultRequestTimeout   = 10 * time.Second
	DefaultPingInterval     = 30 * time.Second
	DefaultUserAgent        = "synthetix-go/wstrade"
)

// Config configures a Client.
type Config struct {
	BaseURL          string
	SubAccountID    uint64
	Signer          *signer.Signer
	DialTimeout     time.Duration
	WriteTimeout    time.Duration
	ReadTimeout     time.Duration
	ReconnectInitial time.Duration
	ReconnectMax     time.Duration
	RequestTimeout    time.Duration
	PingInterval      time.Duration
	UserAgent         string
	Dialer            *websocket.Dialer
	Logger            sdklogger.Logger
}

// Handler is the per-message callback for private push notifications.
type Handler func(msg *types.WSMessage)

// Client manages one authenticated /v1/ws/trade connection.
type Client struct {
	cfg    Config
	wsURL  string
	dialer *websocket.Dialer
	logger sdklogger.Logger

	mu             sync.Mutex
	connReady      *sync.Cond
	conn           *websocket.Conn
	authenticated  bool
	pendingReplies map[string]chan *types.WSMessage
	nextReqID      uint64
	closed         bool
	subscribers    map[uint64]Handler
	nextSubID      uint64

	writeMu sync.Mutex

	runCtx    context.Context
	runCancel context.CancelFunc
	runDone   chan struct{}
}

// NewClient constructs and starts an authenticated trade websocket
// client. Signer and SubAccountID are required because the endpoint
// authenticates on every connection.
func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("wstrade: BaseURL is required")
	}
	if cfg.Signer == nil {
		return nil, errors.New("wstrade: Signer is required")
	}
	if cfg.SubAccountID == 0 {
		return nil, errors.New("wstrade: SubAccountID is required")
	}
	wsURL, err := toWSURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	applyDefaults(&cfg)

	dialer := cfg.Dialer
	if dialer == nil {
		dialer = &websocket.Dialer{HandshakeTimeout: cfg.DialTimeout}
	}
	runCtx, runCancel := context.WithCancel(context.Background())
	c := &Client{
		cfg:            cfg,
		wsURL:          wsURL,
		dialer:         dialer,
		logger:         cfg.Logger,
		pendingReplies: make(map[string]chan *types.WSMessage),
		subscribers:    make(map[uint64]Handler),
		runCtx:         runCtx,
		runCancel:      runCancel,
		runDone:        make(chan struct{}),
	}
	c.connReady = sync.NewCond(&c.mu)
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
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = DefaultRequestTimeout
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

func toWSURL(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("wstrade: invalid BaseURL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "ws":
		u.Scheme = "ws"
	case "https", "wss":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("wstrade: unsupported BaseURL scheme %q", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/ws/trade"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// Close tears down the client. Idempotent.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.subscribers = nil
	c.authenticated = false
	conn := c.conn
	c.connReady.Broadcast()
	c.mu.Unlock()

	c.runCancel()
	if conn != nil {
		_ = conn.Close()
	}
	<-c.runDone
	return nil
}

// Post sends an already-signed REST envelope over /v1/ws/trade and
// decodes the reply result into out when out is non-nil.
func (c *Client) Post(ctx context.Context, signedEnvelope any, out any) error {
	if signedEnvelope == nil {
		return errors.New("wstrade: signedEnvelope is required")
	}
	params, err := flattenSignedEnvelope(signedEnvelope)
	if err != nil {
		return err
	}
	msg, err := c.sendRequest(ctx, "post", params)
	if err != nil {
		return err
	}
	if msg.Error != nil {
		return &WSError{Method: "post", Code: msg.Error.Code, Message: msg.Error.Message}
	}
	if out == nil || len(msg.Result) == 0 || bytes.Equal(msg.Result, []byte("null")) {
		return nil
	}
	result := unwrapPostResult(msg.Result)
	return json.Unmarshal(result, out)
}

// SubscribeSubAccountUpdates subscribes to authenticated account
// updates for one subaccount.
func (c *Client) SubscribeSubAccountUpdates(ctx context.Context, subAccountID uint64, h Handler) (func(), error) {
	if h == nil {
		return nil, errors.New("wstrade: Handler is required")
	}
	if subAccountID == 0 {
		return nil, errors.New("wstrade: subAccountID is required")
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("wstrade: client is closed")
	}
	id := atomic.AddUint64(&c.nextSubID, 1)
	c.subscribers[id] = h
	c.mu.Unlock()

	_, err := c.sendRequest(ctx, "subscribe", map[string]any{
		"type":         "subAccountUpdate",
		"subaccountId": fmt.Sprintf("%d", subAccountID),
	})
	if err != nil {
		c.removeSubscriber(id)
		return nil, err
	}
	return func() {
		c.removeSubscriber(id)
		_, _ = c.sendRequest(context.Background(), "unsubscribe", map[string]any{
			"type":         "subAccountUpdate",
			"subaccountId": fmt.Sprintf("%d", subAccountID),
		})
	}, nil
}

// SubscribeSubAccountUpdatesChan is the channel-oriented form of
// SubscribeSubAccountUpdates. It is the idiomatic Go alternative to
// Python-style async iterators.
func (c *Client) SubscribeSubAccountUpdatesChan(ctx context.Context, subAccountID uint64, buffer int) (<-chan *types.WSMessage, func(), error) {
	if buffer <= 0 {
		buffer = 1
	}
	ch := make(chan *types.WSMessage, buffer)
	unsubscribe, err := c.SubscribeSubAccountUpdates(ctx, subAccountID, func(msg *types.WSMessage) {
		select {
		case ch <- msg:
		default:
			<-ch
			ch <- msg
		}
	})
	if err != nil {
		close(ch)
		return nil, nil, err
	}
	return ch, func() {
		unsubscribe()
		close(ch)
	}, nil
}

func (c *Client) removeSubscriber(id uint64) {
	c.mu.Lock()
	delete(c.subscribers, id)
	c.mu.Unlock()
}

func flattenSignedEnvelope(envelope any) (map[string]any, error) {
	raw, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("wstrade: marshal signed envelope: %w", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("wstrade: decode signed envelope: %w", err)
	}
	params, _ := decoded["params"].(map[string]any)
	if params == nil {
		params = make(map[string]any)
	}
	for k, v := range decoded {
		if k == "params" {
			continue
		}
		params[k] = v
	}
	return params, nil
}

func unwrapPostResult(result json.RawMessage) json.RawMessage {
	var wrapped struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(result, &wrapped); err == nil && len(wrapped.Response) > 0 {
		return wrapped.Response
	}
	return result
}

