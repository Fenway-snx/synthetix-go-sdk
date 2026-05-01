package synthetix

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	sdklogger "github.com/synthetixio/synthetix-go/logger"
	"github.com/synthetixio/synthetix-go/restinfo"
	"github.com/synthetixio/synthetix-go/resttrade"
	"github.com/synthetixio/synthetix-go/signer"
	"github.com/synthetixio/synthetix-go/wstrade"
	"github.com/synthetixio/synthetix-go/wsinfo"
)

// Sane defaults applied when Config fields are zero.
const (
	DefaultBaseURL        = "https://api.synthetix.io"
	DefaultHTTPTimeout    = 10 * time.Second
	DefaultMarketCacheTTL = 30 * time.Second
	DefaultExpiresAfterMs = 60_000
)

const (
	EnvBaseURL       = "SYNTHETIX_BASE_URL"
	EnvPrivateKey    = "SYNTHETIX_PRIVATE_KEY"
	EnvSubAccountID  = "SYNTHETIX_SUB_ACCOUNT_ID"
	EnvExpiresAfterMs = "SYNTHETIX_EXPIRES_AFTER_MS"
)

type AuthMode string

const (
	AuthModeReadOnly AuthMode = "read_only"
	AuthModeTrading  AuthMode = "trading"
)

// Config is the top-level configuration for the Synthetix SDK.
type Config struct {
	// BaseURL of the public Synthetix API. Defaults to DefaultBaseURL.
	BaseURL string

	// HTTPTimeout caps every REST request. Defaults to DefaultHTTPTimeout.
	HTTPTimeout time.Duration

	// MarketCacheTTL controls /v1/info getMarkets caching. Zero uses
	// the SDK default; negative disables caching.
	MarketCacheTTL time.Duration

	// HTTPClient lets callers inject a custom *http.Client (proxies,
	// custom transports, httptest.Server, etc.). When nil, a fresh
	// client honouring HTTPTimeout is constructed.
	HTTPClient *http.Client

	// UserAgent overrides the default header on outbound requests.
	UserAgent string

	// Logger receives SDK observability output. Nil drops silently.
	Logger sdklogger.Logger

	// PrivateKeyHex enables the authenticated trade surface. When
	// non-empty, NewClient parses the key and builds a Signer
	// accessible via Client.Signer(). Leave empty for read-only
	// /v1/info + /v1/ws use; trade-side helpers will return
	// ErrNoSigner.
	PrivateKeyHex string

	// SubAccountID enables the authenticated /v1/ws/trade client when
	// a private key is also configured. Leave unset to allow automatic
	// discovery for high-level helpers.
	SubAccountID uint64

	// WSTradeURL overrides BaseURL for /v1/ws/trade. Empty means use
	// BaseURL.
	WSTradeURL string

	// WSInfoURL overrides BaseURL for /v1/ws/info and WS status
	// checks. Empty means use BaseURL.
	WSInfoURL string

	// AutoDiscoverSubAccount controls whether high-level authenticated
	// helpers fetch the first owned subaccount id when SubAccountID is
	// unset. Defaults to true.
	AutoDiscoverSubAccount bool

	// ExpiresAfterMs is the default signed-action expiry window used by
	// high-level helpers. Zero uses DefaultExpiresAfterMs.
	ExpiresAfterMs int64
}

// Client aggregates the transport clients plus optional auth state.
// Construct via NewClient.
type Client struct {
	info    *restinfo.Client
	trade   *resttrade.Client
	wsInfo  *wsinfo.Client
	wsStatus *restinfo.Client
	wsTrade *wstrade.Client
	signer  *signer.Signer

	baseURL             string
	wsInfoURL           string
	wsTradeURL          string
	userAgent           string
	logger              sdklogger.Logger
	subAccountID       uint64
	autoDiscoverSubAcct bool
	expiresAfterMs      int64
	subAccountMu        sync.Mutex
	wsTradeMu           sync.Mutex
	wsTradeSubAccountID uint64
}

// ErrNoSigner is returned by helpers that require a private key
// when the Client was constructed without one. Use NewSigner or
// pass Config.PrivateKeyHex up front to enable the trade surface.
var ErrNoSigner = errors.New("synthetix: no signer configured (set Config.PrivateKeyHex or call NewSigner)")

// ConfigFromEnv builds Config from SYNTHETIX_* environment variables.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		BaseURL:       os.Getenv(EnvBaseURL),
		PrivateKeyHex: os.Getenv(EnvPrivateKey),
	}
	if raw := os.Getenv(EnvSubAccountID); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("synthetix: parse %s: %w", EnvSubAccountID, err)
		}
		cfg.SubAccountID = id
	}
	if raw := os.Getenv(EnvExpiresAfterMs); raw != "" {
		ms, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("synthetix: parse %s: %w", EnvExpiresAfterMs, err)
		}
		cfg.ExpiresAfterMs = ms
	}
	return cfg, nil
}

// NewClientFromEnv constructs a Client from SYNTHETIX_* environment
// variables.
func NewClientFromEnv() (*Client, error) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return NewClient(cfg)
}

// NewReadOnlyClient constructs a market-data-only client.
func NewReadOnlyClient(baseURL string) (*Client, error) {
	return NewClient(Config{BaseURL: baseURL})
}

// NewTradingClient constructs a trading client and eagerly discovers
// the default subaccount when Config.SubAccountID is unset.
func NewTradingClient(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.PrivateKeyHex == "" {
		return nil, ErrNoSigner
	}
	c, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}
	if _, err := c.DiscoverDefaultSubAccount(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// NewClient builds a Client from cfg. Returns an error only if a downstream
// client constructor rejects the input (e.g. malformed BaseURL).
func NewClient(cfg Config) (*Client, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	timeout := cfg.HTTPTimeout
	if timeout <= 0 {
		timeout = DefaultHTTPTimeout
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = sdklogger.Nop()
	}

	info, err := restinfo.NewClient(restinfo.Config{
		BaseURL:        baseURL,
		HTTPTimeout:    timeout,
		MarketCacheTTL: cfg.MarketCacheTTL,
		HTTPClient:     httpClient,
		UserAgent:      cfg.UserAgent,
		Logger:         logger,
	})
	if err != nil {
		return nil, fmt.Errorf("synthetix: build info client: %w", err)
	}

	trade, err := resttrade.NewClient(resttrade.Config{
		BaseURL:     baseURL,
		HTTPTimeout: timeout,
		HTTPClient:  httpClient,
		UserAgent:   cfg.UserAgent,
		Logger:      logger,
	})
	if err != nil {
		return nil, fmt.Errorf("synthetix: build trade client: %w", err)
	}

	wsInfoURL := cfg.WSInfoURL
	if wsInfoURL == "" {
		wsInfoURL = baseURL
	}
	ws, err := wsinfo.NewClient(wsinfo.Config{
		BaseURL: wsInfoURL,
		Logger:  logger,
	})
	if err != nil {
		return nil, fmt.Errorf("synthetix: build ws client: %w", err)
	}
	wsStatus, err := restinfo.NewClient(restinfo.Config{
		BaseURL:        wsInfoURL,
		HTTPTimeout:    timeout,
		MarketCacheTTL: cfg.MarketCacheTTL,
		HTTPClient:     httpClient,
		UserAgent:      cfg.UserAgent,
		Logger:         logger,
	})
	if err != nil {
		return nil, fmt.Errorf("synthetix: build ws status client: %w", err)
	}

	expiresAfterMs := cfg.ExpiresAfterMs
	if expiresAfterMs == 0 {
		expiresAfterMs = DefaultExpiresAfterMs
	}
	c := &Client{
		info:                info,
		trade:               trade,
		wsInfo:              ws,
		wsStatus:            wsStatus,
		baseURL:             baseURL,
		wsInfoURL:           wsInfoURL,
		wsTradeURL:          cfg.WSTradeURL,
		userAgent:           cfg.UserAgent,
		logger:              logger,
		subAccountID:        cfg.SubAccountID,
		autoDiscoverSubAcct: cfg.AutoDiscoverSubAccount || cfg.SubAccountID == 0,
		expiresAfterMs:      expiresAfterMs,
	}
	if cfg.PrivateKeyHex != "" {
		s, err := signer.New(cfg.PrivateKeyHex)
		if err != nil {
			return nil, fmt.Errorf("synthetix: build signer: %w", err)
		}
		c.signer = s
		if cfg.SubAccountID != 0 {
			if err := c.ensureWSTrade(cfg.SubAccountID); err != nil {
				return nil, fmt.Errorf("synthetix: build trade websocket client: %w", err)
			}
		}
	}
	return c, nil
}

// Returns the read-only /v1/info client.
func (c *Client) Info() *restinfo.Client { return c.info }

// Returns the authenticated /v1/trade client.
func (c *Client) Trade() *resttrade.Client { return c.trade }

// WSInfo returns the public streaming WebSocket client.
func (c *Client) WSInfo() *wsinfo.Client { return c.wsInfo }

// WS returns the public streaming WebSocket client.
//
// Deprecated: use WSInfo. WSTrade returns the authenticated trade
// websocket client.
func (c *Client) WS() *wsinfo.Client { return c.WSInfo() }

// WSTrade returns the authenticated /v1/ws/trade client, or nil when
// the client was constructed without PrivateKeyHex and SubAccountID.
func (c *Client) WSTrade() *wstrade.Client { return c.wsTrade }

// Signer returns the signer attached to this Client. Nil if the
// Client was constructed without a private key. Use HasSigner first
// to disambiguate.
func (c *Client) Signer() *signer.Signer { return c.signer }

// HasSigner reports whether the Client was constructed with a
// private key (or had one attached later via NewSigner).
func (c *Client) HasSigner() bool { return c.signer != nil }

// NewSigner attaches (or replaces) the Signer used for the
// authenticated trade surface. Useful when the private key isn't
// known at NewClient time (e.g. lazy KMS load). Returns an error
// if the key cannot be parsed.
func (c *Client) NewSigner(privateKeyHex string) error {
	s, err := signer.New(privateKeyHex)
	if err != nil {
		return err
	}
	c.signer = s
	if c.subAccountID != 0 {
		return c.ensureWSTrade(c.subAccountID)
	}
	return nil
}

// WalletAddress returns the 0x-prefixed checksum address derived
// from the attached signer's private key. Returns ErrNoSigner if
// no signer is configured.
func (c *Client) WalletAddress() (string, error) {
	if c.signer == nil {
		return "", ErrNoSigner
	}
	return c.signer.WalletAddress(), nil
}

// AuthMode reports whether this client is read-only or trading-capable.
func (c *Client) AuthMode() AuthMode {
	if c != nil && c.signer != nil {
		return AuthModeTrading
	}
	return AuthModeReadOnly
}

// DefaultSubAccountID returns the cached default subaccount id.
func (c *Client) DefaultSubAccountID() (uint64, bool) {
	c.subAccountMu.Lock()
	defer c.subAccountMu.Unlock()
	return c.subAccountID, c.subAccountID != 0
}

// SetDefaultSubAccountID sets the default subaccount and initializes
// authenticated websocket trading when a signer is configured.
func (c *Client) SetDefaultSubAccountID(id uint64) error {
	if id == 0 {
		return errors.New("synthetix: subaccount id is required")
	}
	c.subAccountMu.Lock()
	c.subAccountID = id
	c.subAccountMu.Unlock()
	if c.signer != nil {
		return c.ensureWSTrade(id)
	}
	return nil
}

// DiscoverDefaultSubAccount discovers and caches the first owned
// subaccount for the configured wallet.
func (c *Client) DiscoverDefaultSubAccount(ctx context.Context) (uint64, error) {
	o := c.options()
	return c.resolveSubAccount(ctx, o)
}

func (c *Client) ensureWSTrade(subAccountID uint64) error {
	if subAccountID == 0 || c.signer == nil {
		return nil
	}
	c.wsTradeMu.Lock()
	defer c.wsTradeMu.Unlock()
	if c.wsTrade != nil && c.wsTradeSubAccountID == subAccountID {
		return nil
	}
	if c.wsTrade != nil {
		_ = c.wsTrade.Close()
		c.wsTrade = nil
		c.wsTradeSubAccountID = 0
	}
	wsTradeURL := c.wsTradeURL
	if wsTradeURL == "" {
		wsTradeURL = c.baseURL
	}
	wt, err := wstrade.NewClient(wstrade.Config{
		BaseURL:       wsTradeURL,
		SubAccountID: subAccountID,
		Signer:       c.signer,
		UserAgent:    c.userAgent,
		Logger:       c.logger,
	})
	if err != nil {
		return err
	}
	c.wsTrade = wt
	c.wsTradeSubAccountID = subAccountID
	return nil
}

// Close releases websocket connections. The REST clients hold no
// long-lived resources.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	var err error
	if c.wsInfo != nil {
		err = c.wsInfo.Close()
	}
	if c.wsTrade != nil {
		if closeErr := c.wsTrade.Close(); err == nil {
			err = closeErr
		}
		c.wsTrade = nil
		c.wsTradeSubAccountID = 0
	}
	return err
}
