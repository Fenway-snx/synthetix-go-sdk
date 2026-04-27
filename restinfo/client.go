// Package restinfo is a typed client for the public Synthetix V4 REST
// /v1/info endpoint.
//
// The client handles error envelope parsing, context-scoped timeouts,
// structured errors, and test seams via httptest.
//
// The client is deliberately stateless except for the GetMarket TTL
// cache. All methods are safe for concurrent use from multiple
// goroutines; the underlying *http.Client and sync.RWMutex guard
// against races.
//
// Every /v1/info action is a POST to BaseURL+"/v1/info" with a JSON
// body of the shape {"action": "...", ...params}. The response is an
// APIResponse[T] envelope; on transport success with a non-nil
// Error branch, we surface that as a *RESTError so callers can
// distinguish "API said no" from "network exploded".
package restinfo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	sdklogger "github.com/Fenway-snx/synthetix-go-sdk/logger"
	"github.com/Fenway-snx/synthetix-go-sdk/types"
)

// DefaultHTTPTimeout is applied when Config.HTTPTimeout is zero.
const DefaultHTTPTimeout = 10 * time.Second

// DefaultMarketCacheTTL is applied when Config.MarketCacheTTL is zero.
// Chosen to balance freshness against bandwidth; market configs
// change on the order of hours, not seconds, so 30s is generous.
const DefaultMarketCacheTTL = 30 * time.Second

// DefaultUserAgent is the User-Agent header sent on every request
// when Config.UserAgent is empty.
const DefaultUserAgent = "synthetix-go/restinfo"

// Config configures a Client.
type Config struct {
	// BaseURL is the Synthetix API root, e.g. "https://api.synthetix.io"
	// or "http://localhost:8080". Required.
	BaseURL string

	// HTTPTimeout caps a single request's total round-trip.
	// Defaults to DefaultHTTPTimeout when zero.
	HTTPTimeout time.Duration

	// MarketCacheTTL controls how long GetMarket caches the full
	// getMarkets response. Defaults to DefaultMarketCacheTTL when zero.
	// Set to a negative value to disable caching entirely.
	MarketCacheTTL time.Duration

	// UserAgent overrides DefaultUserAgent.
	UserAgent string

	// HTTPClient lets tests inject an *http.Client backed by an
	// httptest.Server. If nil, a fresh client with HTTPTimeout is used.
	HTTPClient *http.Client

	// Logger is the structured logger for client-internal
	// observability. Nil is allowed (logs drop silently).
	Logger sdklogger.Logger
}

// Client is the /v1/info typed client.
type Client struct {
	baseURL        string
	httpClient     *http.Client
	userAgent      string
	marketCacheTTL time.Duration
	logger         sdklogger.Logger

	// marketCache caches the latest getMarkets response. Exposed via
	// GetMarket(symbol). Concurrent reads use the RWMutex.
	marketCache struct {
		mu        sync.RWMutex
		markets   []types.MarketResponse
		populated bool
		fetchedAt time.Time
	}
}

// NewClient builds a Client from a Config. Returns an error only
// when BaseURL is empty or malformed; all other fields take
// documented defaults.
func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("restinfo: BaseURL is required")
	}
	if _, err := url.Parse(cfg.BaseURL); err != nil {
		return nil, fmt.Errorf("restinfo: invalid BaseURL: %w", err)
	}

	timeout := cfg.HTTPTimeout
	if timeout <= 0 {
		timeout = DefaultHTTPTimeout
	}

	cacheTTL := cfg.MarketCacheTTL
	if cacheTTL == 0 {
		cacheTTL = DefaultMarketCacheTTL
	}

	ua := cfg.UserAgent
	if ua == "" {
		ua = DefaultUserAgent
	}

	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: timeout}
	}

	return &Client{
		baseURL:        strings.TrimRight(cfg.BaseURL, "/"),
		httpClient:     hc,
		userAgent:      ua,
		marketCacheTTL: cacheTTL,
		logger:         cfg.Logger,
	}, nil
}

// ---------------------------------------------------------------------
// Public action methods
// ---------------------------------------------------------------------

// GetExchangeStatus calls the dedicated REST exchange status endpoint.
func (c *Client) GetExchangeStatus(ctx context.Context) (*types.ExchangeStatusResponse, error) {
	var out types.ExchangeStatusResponse
	if err := c.getStatus(ctx, "/v1/exchange/status", "getExchangeStatus", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetWSExchangeStatus calls the dedicated WebSocket exchange status endpoint.
func (c *Client) GetWSExchangeStatus(ctx context.Context) (*types.ExchangeStatusResponse, error) {
	var out types.ExchangeStatusResponse
	if err := c.getStatus(ctx, "/v1/ws/exchange/status", "getWSExchangeStatus", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetMarkets calls /v1/info getMarkets and returns the full market
// config list. Used by context_tools and as the backing fetch for
// GetMarket(symbol) via the TTL cache.
//
// The activeOnly flag filters to open markets when true.
func (c *Client) GetMarkets(ctx context.Context, activeOnly bool) ([]types.MarketResponse, error) {
	var out []types.MarketResponse
	req := infoRequest{Action: "getMarkets"}
	if activeOnly {
		req.Extra = map[string]any{"activeOnly": true}
	}
	if err := c.postInfo(ctx, req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetMarket returns a single market by symbol. Under the hood it
// calls GetMarkets and filters; the full list is cached for
// MarketCacheTTL to amortize the bandwidth cost across repeated
// single-market lookups from tools like get_market_summary and the
// market://specs/{symbol} resource.
//
// Returns ErrMarketNotFound if the symbol is not present in the
// cached snapshot. Callers that want to trigger a refetch can invoke
// InvalidateMarketCache.
func (c *Client) GetMarket(ctx context.Context, symbol string) (*types.MarketResponse, error) {
	if symbol == "" {
		return nil, errors.New("restinfo: symbol is required")
	}

	markets, err := c.marketsCached(ctx)
	if err != nil {
		return nil, err
	}
	for i := range markets {
		if markets[i].Symbol == symbol {
			m := markets[i]
			return &m, nil
		}
	}
	return nil, ErrMarketNotFound{Symbol: symbol}
}

// InvalidateMarketCache drops the cached getMarkets response so the
// next GetMarket / cached GetMarkets call re-fetches.
func (c *Client) InvalidateMarketCache() {
	c.marketCache.mu.Lock()
	defer c.marketCache.mu.Unlock()
	c.marketCache.populated = false
	c.marketCache.markets = nil
}

// GetMarketPrices calls /v1/info getMarketPrices and returns the
// current ticker map keyed by symbol.
func (c *Client) GetMarketPrices(ctx context.Context) (map[string]types.MarketPriceResponse, error) {
	out := make(map[string]types.MarketPriceResponse)
	if err := c.postInfo(ctx, infoRequest{Action: "getMarketPrices"}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetOrderbook calls /v1/info getOrderbook for one symbol.
// limit=0 means use the API default depth.
func (c *Client) GetOrderbook(ctx context.Context, symbol string, limit int) (*types.OrderbookResponse, error) {
	if symbol == "" {
		return nil, errors.New("restinfo: symbol is required")
	}
	extra := map[string]any{"symbol": symbol}
	if limit > 0 {
		extra["limit"] = limit
	}
	var out types.OrderbookResponse
	if err := c.postInfo(ctx, infoRequest{Action: "getOrderbook", Extra: extra}, &out); err != nil {
		return nil, err
	}
	if out.Symbol == "" {
		out.Symbol = symbol
	}
	return &out, nil
}

// GetCandles calls /v1/info getCandles. interval is required;
// limit=0 / startTimeMs=0 / endTimeMs=0 mean "unset" and are omitted
// from the request body so the API applies its own defaults.
func (c *Client) GetCandles(
	ctx context.Context,
	symbol, interval string,
	limit int,
	startTimeMs, endTimeMs int64,
) (*types.CandleResponse, error) {
	if symbol == "" {
		return nil, errors.New("restinfo: symbol is required")
	}
	if interval == "" {
		return nil, errors.New("restinfo: interval is required")
	}
	extra := map[string]any{
		"symbol":   symbol,
		"interval": interval,
	}
	if limit > 0 {
		extra["limit"] = limit
	}
	if startTimeMs > 0 {
		extra["startTime"] = startTimeMs
	}
	if endTimeMs > 0 {
		extra["endTime"] = endTimeMs
	}
	var out types.CandleResponse
	if err := c.postInfo(ctx, infoRequest{Action: "getCandles", Extra: extra}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetLastTrades calls /v1/info getLastTrades. limit and offset of 0
// mean "unset" and are omitted.
func (c *Client) GetLastTrades(ctx context.Context, symbol string, limit, offset int) (*types.LastTradesResponse, error) {
	if symbol == "" {
		return nil, errors.New("restinfo: symbol is required")
	}
	extra := map[string]any{"symbol": symbol}
	if limit > 0 {
		extra["limit"] = limit
	}
	if offset > 0 {
		extra["offset"] = offset
	}
	var out types.LastTradesResponse
	if err := c.postInfo(ctx, infoRequest{Action: "getLastTrades", Extra: extra}, &out); err != nil {
		return nil, err
	}
	if out.Symbol == "" {
		out.Symbol = symbol
	}
	return &out, nil
}

// GetFundingRate calls /v1/info getFundingRate for one symbol.
func (c *Client) GetFundingRate(ctx context.Context, symbol string) (*types.FundingRateResponse, error) {
	if symbol == "" {
		return nil, errors.New("restinfo: symbol is required")
	}
	extra := map[string]any{"symbol": symbol}
	var out types.FundingRateResponse
	if err := c.postInfo(ctx, infoRequest{Action: "getFundingRate", Extra: extra}, &out); err != nil {
		return nil, err
	}
	if out.Symbol == "" {
		out.Symbol = symbol
	}
	return &out, nil
}

// GetFundingRateHistory calls /v1/info getFundingRateHistory for one
// symbol. limit=0 / startTimeMs=0 / endTimeMs=0 mean "unset" and are
// omitted so the API applies its defaults.
func (c *Client) GetFundingRateHistory(
	ctx context.Context,
	symbol string,
	limit int,
	startTimeMs, endTimeMs int64,
) (*types.FundingRateHistoryResponse, error) {
	if symbol == "" {
		return nil, errors.New("restinfo: symbol is required")
	}
	extra := map[string]any{"symbol": symbol}
	if limit > 0 {
		extra["limit"] = limit
	}
	if startTimeMs > 0 {
		extra["startTime"] = startTimeMs
	}
	if endTimeMs > 0 {
		extra["endTime"] = endTimeMs
	}
	var out types.FundingRateHistoryResponse
	if err := c.postInfo(ctx, infoRequest{Action: "getFundingRateHistory", Extra: extra}, &out); err != nil {
		return nil, err
	}
	if out.Symbol == "" {
		out.Symbol = symbol
	}
	return &out, nil
}

// GetOpenInterest calls /v1/info getOpenInterest and returns the
// snapshot across all symbols.
func (c *Client) GetOpenInterest(ctx context.Context) ([]types.OpenInterestEntry, error) {
	var out []types.OpenInterestEntry
	if err := c.postInfo(ctx, infoRequest{Action: "getOpenInterest"}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetMids calls /v1/info getMids and returns a map of symbol to
// mid price.
func (c *Client) GetMids(ctx context.Context) (map[string]types.MidResponse, error) {
	out := make(map[string]types.MidResponse)
	if err := c.postInfo(ctx, infoRequest{Action: "getMids"}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetSubAccountIds calls /v1/info getSubAccountIds for the given
// wallet. When includeDelegations is false, the response is a flat
// list of owned ids. When true, the response is a struct with both
// owned and delegated id lists.
//
// We expose two typed methods so callers don't have to type-assert.
func (c *Client) GetSubAccountIds(ctx context.Context, walletAddress string) ([]string, error) {
	if walletAddress == "" {
		return nil, errors.New("restinfo: walletAddress is required")
	}
	extra := map[string]any{"walletAddress": walletAddress}
	var out types.SubAccountIdsResponse
	if err := c.postInfo(ctx, infoRequest{Action: "getSubAccountIds", Extra: extra}, &out); err != nil {
		return nil, err
	}
	return out.IDs, nil
}

// GetSubAccountIdsWithDelegations calls getSubAccountIds with
// includeDelegations=true and returns both owned and delegated id
// lists.
func (c *Client) GetSubAccountIdsWithDelegations(
	ctx context.Context,
	walletAddress string,
) (*types.SubAccountIdsWithDelegationsResponse, error) {
	if walletAddress == "" {
		return nil, errors.New("restinfo: walletAddress is required")
	}
	extra := map[string]any{
		"walletAddress":      walletAddress,
		"includeDelegations": true,
	}
	var out types.SubAccountIdsWithDelegationsResponse
	if err := c.postInfo(ctx, infoRequest{Action: "getSubAccountIds", Extra: extra}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------------------------------------------------------------
// Internal plumbing: request envelope, HTTP transport, error handling
// ---------------------------------------------------------------------

// infoRequest is the JSON body shape of every /v1/info POST.
// Extra is flattened into the top-level object at encode time so the
// wire format is {"action":"...", ...extra}.
type infoRequest struct {
	Action string
	Extra  map[string]any
}

// MarshalJSON flattens Extra next to Action so the output matches the
// public request body: {"action": "...", "symbol": "...", ...}.
func (r infoRequest) MarshalJSON() ([]byte, error) {
	merged := make(map[string]any, len(r.Extra)+1)
	for k, v := range r.Extra {
		merged[k] = v
	}
	merged["action"] = r.Action
	return json.Marshal(merged)
}

// postInfo is the single point that every public action method goes
// through. It builds the request, sends it, parses the APIResponse
// envelope, and deserializes Response into out on success.
//
// Transport errors (DNS, TCP, TLS, read/write, HTTP >= 500 with no
// structured body) surface as *TransportError. Structured API errors
// (HTTP >= 400 with a parseable error body, or 2xx with a non-nil
// error branch) surface as *RESTError.
func (c *Client) postInfo(ctx context.Context, req infoRequest, out any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("restinfo: marshal %s request: %w", req.Action, err)
	}

	url := c.baseURL + "/v1/info"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("restinfo: build %s request: %w", req.Action, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return &TransportError{Action: req.Action, Err: err}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &TransportError{Action: req.Action, StatusCode: resp.StatusCode, Err: fmt.Errorf("read response body: %w", err)}
	}

	// Try to parse the envelope. If the server returned a non-JSON
	// body (e.g. HTML 5xx page from a load balancer), surface that
	// as a transport error with the raw bytes.
	var envelope types.RawAPIResponse
	if decodeErr := json.Unmarshal(respBody, &envelope); decodeErr != nil {
		return &TransportError{
			Action:       req.Action,
			StatusCode:   resp.StatusCode,
			Err:          fmt.Errorf("decode envelope: %w", decodeErr),
			RawBodyBytes: respBody,
		}
	}

	if envelope.Error != nil {
		return &RESTError{
			Action:     req.Action,
			StatusCode: resp.StatusCode,
			RequestID:  envelope.RequestID,
			Code:       envelope.Error.Code,
			Message:    envelope.Error.Message,
			Details:    envelope.Error.Details,
		}
	}

	if resp.StatusCode >= 400 {
		return &TransportError{
			Action:       req.Action,
			StatusCode:   resp.StatusCode,
			Err:          fmt.Errorf("http %d with no error envelope", resp.StatusCode),
			RawBodyBytes: respBody,
		}
	}

	if out == nil {
		return nil
	}
	if len(envelope.Response) == 0 || bytes.Equal(envelope.Response, []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(envelope.Response, out); err != nil {
		return fmt.Errorf("restinfo: decode %s response: %w", req.Action, err)
	}
	return nil
}

func (c *Client) getStatus(ctx context.Context, path, action string, out any) error {
	url := c.baseURL + path
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("restinfo: build %s request: %w", action, err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return &TransportError{Action: action, Err: err}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &TransportError{Action: action, StatusCode: resp.StatusCode, Err: fmt.Errorf("read response body: %w", err)}
	}

	if err := decodeStatusResponse(respBody, out); err != nil {
		return &TransportError{
			Action:       action,
			StatusCode:   resp.StatusCode,
			Err:          err,
			RawBodyBytes: respBody,
		}
	}
	if resp.StatusCode >= 400 {
		return &TransportError{
			Action:       action,
			StatusCode:   resp.StatusCode,
			Err:          fmt.Errorf("http %d with no error envelope", resp.StatusCode),
			RawBodyBytes: respBody,
		}
	}
	return nil
}

func decodeStatusResponse(body []byte, out any) error {
	var envelope types.RawAPIResponse
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Response) > 0 && !bytes.Equal(envelope.Response, []byte("null")) {
		return json.Unmarshal(envelope.Response, out)
	}
	return json.Unmarshal(body, out)
}

// marketsCached returns the cached getMarkets snapshot if it's still
// fresh, or fetches and caches a new one. Concurrent callers on a
// cold cache all wait on the same in-flight request via a write lock;
// this prevents a thundering herd after cache expiry.
func (c *Client) marketsCached(ctx context.Context) ([]types.MarketResponse, error) {
	if c.marketCacheTTL < 0 {
		return c.GetMarkets(ctx, false)
	}

	c.marketCache.mu.RLock()
	fresh := c.marketCache.populated && time.Since(c.marketCache.fetchedAt) < c.marketCacheTTL
	if fresh {
		snapshot := c.marketCache.markets
		c.marketCache.mu.RUnlock()
		return snapshot, nil
	}
	c.marketCache.mu.RUnlock()

	c.marketCache.mu.Lock()
	defer c.marketCache.mu.Unlock()
	// Re-check under write lock in case another goroutine
	// populated while we were waiting.
	if c.marketCache.populated && time.Since(c.marketCache.fetchedAt) < c.marketCacheTTL {
		return c.marketCache.markets, nil
	}

	markets, err := c.GetMarkets(ctx, false)
	if err != nil {
		return nil, err
	}
	c.marketCache.markets = markets
	c.marketCache.populated = true
	c.marketCache.fetchedAt = time.Now()
	return markets, nil
}

// ---------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------

// TransportError is returned for network-layer failures and for
// responses that cannot be parsed as an APIResponse envelope.
type TransportError struct {
	Action       string
	StatusCode   int
	Err          error
	RawBodyBytes []byte
}

func (e *TransportError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("restinfo: transport error on %s (status %d): %v", e.Action, e.StatusCode, e.Err)
	}
	return fmt.Sprintf("restinfo: transport error on %s: %v", e.Action, e.Err)
}

func (e *TransportError) Unwrap() error { return e.Err }

// RESTError is returned when the API replies with a structured
// error branch. The Code and Message fields come straight from the
// JSON envelope; Details may be any JSON value the server attaches
// (validation details, etc).
type RESTError struct {
	Action     string
	StatusCode int
	RequestID  string
	Code       string
	Message    string
	Details    json.RawMessage
}

func (e *RESTError) Error() string {
	return fmt.Sprintf("restinfo: %s returned API error %s (status %d): %s", e.Action, e.Code, e.StatusCode, e.Message)
}

// ErrMarketNotFound is returned by GetMarket when the symbol is not
// present in the cached snapshot. Callers can use errors.As to
// recover the symbol that was requested.
type ErrMarketNotFound struct {
	Symbol string
}

func (e ErrMarketNotFound) Error() string {
	return fmt.Sprintf("restinfo: market %q not found", e.Symbol)
}
