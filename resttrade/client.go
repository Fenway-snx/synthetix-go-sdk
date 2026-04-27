// Package resttrade is a typed client for the public Synthetix V4
// REST /v1/trade endpoint.
//
// This client is a pure HTTP transport. It accepts *already-signed*
// request envelopes and does NOT hold, load, or use private key
// material. Keeping signing separate lets callers use the SDK signer,
// a hardware wallet, KMS, or another external signer.
//
// The client is safe for concurrent use from multiple goroutines; the
// embedded *http.Client is the only shared mutable state.
package resttrade

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
	"time"

	sdklogger "github.com/Fenway-snx/synthetix-go-sdk/logger"
	"github.com/Fenway-snx/synthetix-go-sdk/types"
)

// DefaultHTTPTimeout is applied when Config.HTTPTimeout is zero.
// Trade writes are user-facing and must surface network failures
// fast; 10s matches restinfo.DefaultHTTPTimeout.
const DefaultHTTPTimeout = 10 * time.Second

// DefaultUserAgent is the User-Agent header sent on every request
// when Config.UserAgent is empty.
const DefaultUserAgent = "synthetix-go/resttrade"

// Config configures a Client.
type Config struct {
	// BaseURL is the Synthetix API root, e.g. "https://api.synthetix.io"
	// or "http://localhost:8080". Required.
	BaseURL string

	// HTTPTimeout caps a single request's total round-trip. Defaults
	// to DefaultHTTPTimeout when zero.
	HTTPTimeout time.Duration

	// UserAgent overrides DefaultUserAgent.
	UserAgent string

	// HTTPClient lets tests inject an *http.Client backed by an
	// httptest.Server. If nil, a fresh client with HTTPTimeout is used.
	HTTPClient *http.Client

	// Logger is the structured logger for client-internal
	// observability. Nil is allowed (logs drop silently).
	Logger sdklogger.Logger
}

// Client is the /v1/trade typed client.
type Client struct {
	baseURL    string
	httpClient *http.Client
	userAgent  string
	logger     sdklogger.Logger
}

// NewClient builds a Client from a Config. Returns an error only
// when BaseURL is empty or malformed.
func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("resttrade: BaseURL is required")
	}
	if _, err := url.Parse(cfg.BaseURL); err != nil {
		return nil, fmt.Errorf("resttrade: invalid BaseURL: %w", err)
	}

	timeout := cfg.HTTPTimeout
	if timeout <= 0 {
		timeout = DefaultHTTPTimeout
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
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		httpClient: hc,
		userAgent:  ua,
		logger:     cfg.Logger,
	}, nil
}

// ---------------------------------------------------------------------
// Order lifecycle (signed writes)
// ---------------------------------------------------------------------

// PlaceOrders POSTs a signed placeOrders envelope and returns the
// typed response. The envelope's Params field must already be
// populated with the action payload whose bytes were used to build
// the EIP-712 signature. resttrade neither synthesises an "action"
// field nor inspects the payload.
func (c *Client) PlaceOrders(ctx context.Context, req *types.PlaceOrdersRequest) (*types.PlaceOrdersResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.PlaceOrdersResponse
	if err := c.postTrade(ctx, "placeOrders", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifyOrder POSTs a signed modifyOrder envelope.
func (c *Client) ModifyOrder(ctx context.Context, req *types.ModifyOrderRequest) (*types.ModifyOrderResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.ModifyOrderResponse
	if err := c.postTrade(ctx, "modifyOrder", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelOrders POSTs a signed cancelOrders envelope.
func (c *Client) CancelOrders(ctx context.Context, req *types.CancelOrdersRequest) (*types.CancelOrdersResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.CancelOrdersResponse
	if err := c.postTrade(ctx, "cancelOrders", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelOrdersByCloid POSTs a signed cancelOrders envelope carrying
// clientOrderIds rather than venue orderIds.
func (c *Client) CancelOrdersByCloid(ctx context.Context, req *types.CancelOrdersRequest) (*types.CancelOrdersResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.CancelOrdersResponse
	if err := c.postTrade(ctx, "cancelOrders", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelAllOrders POSTs a signed cancelAllOrders envelope.
func (c *Client) CancelAllOrders(ctx context.Context, req *types.CancelAllOrdersRequest) (*types.CancelAllOrdersResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.CancelAllOrdersResponse
	if err := c.postTrade(ctx, "cancelAllOrders", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateLeverage POSTs a signed updateLeverage envelope.
func (c *Client) UpdateLeverage(ctx context.Context, req *types.UpdateLeverageRequest) (*types.UpdateLeverageResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.UpdateLeverageResponse
	if err := c.postTrade(ctx, "updateLeverage", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WithdrawCollateral POSTs a signed withdrawCollateral envelope.
func (c *Client) WithdrawCollateral(ctx context.Context, req *types.WithdrawCollateralRequest) (*types.WithdrawCollateralResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.WithdrawCollateralResponse
	if err := c.postTrade(ctx, "withdrawCollateral", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TransferCollateral POSTs a signed transferCollateral envelope.
func (c *Client) TransferCollateral(ctx context.Context, req *types.TransferCollateralRequest) (*types.TransferCollateralResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.TransferCollateralResponse
	if err := c.postTrade(ctx, "transferCollateral", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ScheduleCancel POSTs a signed scheduleCancel envelope.
func (c *Client) ScheduleCancel(ctx context.Context, req *types.ScheduleCancelRequest) (*types.ScheduleCancelResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.ScheduleCancelResponse
	if err := c.postTrade(ctx, "scheduleCancel", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------------------------------------------------------------
// Authenticated reads (signed but idempotent)
// ---------------------------------------------------------------------

// GetSubAccount POSTs a signed getSubAccount envelope and returns
// the single subaccount snapshot for the signed-for account. The
// public wire shape returns a single SubAccountResponse, not a list.
func (c *Client) GetSubAccount(ctx context.Context, req *types.SubAccountActionRequest) (*types.SubAccountResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.SubAccountResponse
	if err := c.postTrade(ctx, "getSubAccount", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetSubAccounts POSTs a signed getSubAccounts envelope. The response
// includes delegate signers for each subaccount.
func (c *Client) GetSubAccounts(ctx context.Context, req *types.SubAccountActionRequest) ([]types.SubAccountWithDelegatesResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	// The API wraps the list in an envelope: {"subAccounts": [...]}.
	var wrapper struct {
		SubAccounts []types.SubAccountWithDelegatesResponse `json:"subAccounts"`
	}
	if err := c.postTrade(ctx, "getSubAccounts", req, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.SubAccounts, nil
}

// GetOpenOrders POSTs a signed getOpenOrders envelope.
func (c *Client) GetOpenOrders(ctx context.Context, req *types.SubAccountActionRequest) ([]types.OpenOrder, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out []types.OpenOrder
	if err := c.postTrade(ctx, "getOpenOrders", req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPositions POSTs a signed getPositions envelope.
func (c *Client) GetPositions(ctx context.Context, req *types.SubAccountActionRequest) ([]types.Position, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out []types.Position
	if err := c.postTrade(ctx, "getPositions", req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetOrderHistory POSTs a signed getOrderHistory envelope. The wire
// shape is a bare JSON array, so the decoded response is a slice of
// history items, not a wrapped object. Filtering
// (symbol, status, time range, client order id) belongs in the
// params map the caller threads through req.Params; this client is
// a pure transport and does not synthesize filter defaults.
func (c *Client) GetOrderHistory(ctx context.Context, req *types.SubAccountActionRequest) (types.OrderHistoryResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.OrderHistoryResponse
	if err := c.postTrade(ctx, "getOrderHistory", req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetTrades POSTs a signed getTrades envelope. Unlike
// GetOrderHistory, the API returns a wrapped response with trades +
// hasMore + total, so callers paginate against `Total`
// rather than counting returned rows. `Total` is the full filter-
// scoped count, not the current-page length.
func (c *Client) GetTrades(ctx context.Context, req *types.SubAccountActionRequest) (*types.TradeHistoryResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.TradeHistoryResponse
	if err := c.postTrade(ctx, "getTrades", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetFundingPayments POSTs a signed getFundingPayments envelope.
// Returns the summary + full payment history per filter; there is no
// pagination envelope on the wire.
func (c *Client) GetFundingPayments(ctx context.Context, req *types.SubAccountActionRequest) (*types.FundingPaymentsResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.FundingPaymentsResponse
	if err := c.postTrade(ctx, "getFundingPayments", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPerformanceHistory POSTs a signed getPerformanceHistory
// envelope. The API defaults the period to "day" when the caller omits
// it; this client does not second-guess that default,
// so a request with empty Params yields the day-period history.
func (c *Client) GetPerformanceHistory(ctx context.Context, req *types.SubAccountActionRequest) (*types.PerformanceHistoryResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.PerformanceHistoryResponse
	if err := c.postTrade(ctx, "getPerformanceHistory", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetBalanceUpdates POSTs a signed getBalanceUpdates envelope.
func (c *Client) GetBalanceUpdates(ctx context.Context, req *types.SubAccountActionRequest) (types.BalanceUpdatesResponse, error) {
	if req == nil {
		return types.BalanceUpdatesResponse{}, errors.New("resttrade: request is required")
	}
	var out types.BalanceUpdatesResponse
	if err := c.postTrade(ctx, "getBalanceUpdates", req, &out); err != nil {
		return types.BalanceUpdatesResponse{}, err
	}
	return out, nil
}

// GetTransfers POSTs a signed getTransfers envelope.
func (c *Client) GetTransfers(ctx context.Context, req *types.SubAccountActionRequest) (types.TransfersResponse, error) {
	if req == nil {
		return types.TransfersResponse{}, errors.New("resttrade: request is required")
	}
	var out types.TransfersResponse
	if err := c.postTrade(ctx, "getTransfers", req, &out); err != nil {
		return types.TransfersResponse{}, err
	}
	return out, nil
}

// GetPositionHistory POSTs a signed getPositionHistory envelope.
func (c *Client) GetPositionHistory(ctx context.Context, req *types.SubAccountActionRequest) (types.PositionHistoryResponse, error) {
	if req == nil {
		return types.PositionHistoryResponse{}, errors.New("resttrade: request is required")
	}
	var out types.PositionHistoryResponse
	if err := c.postTrade(ctx, "getPositionHistory", req, &out); err != nil {
		return types.PositionHistoryResponse{}, err
	}
	return out, nil
}

// GetPortfolio POSTs a signed getPortfolio envelope.
func (c *Client) GetPortfolio(ctx context.Context, req *types.SubAccountActionRequest) (*types.PortfolioResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.PortfolioResponse
	if err := c.postTrade(ctx, "getPortfolio", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTradesForPosition POSTs a signed getTradesForPosition envelope.
func (c *Client) GetTradesForPosition(ctx context.Context, req *types.SubAccountActionRequest) (*types.TradesForPositionResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.TradesForPositionResponse
	if err := c.postTrade(ctx, "getTradesForPosition", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetFees POSTs a signed getFees envelope.
func (c *Client) GetFees(ctx context.Context, req *types.SubAccountActionRequest) (*types.FeesResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.FeesResponse
	if err := c.postTrade(ctx, "getFees", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetRateLimits POSTs a signed getRateLimits envelope.
func (c *Client) GetRateLimits(ctx context.Context, req *types.SubAccountActionRequest) (*types.RateLimitsResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.RateLimitsResponse
	if err := c.postTrade(ctx, "getRateLimits", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetDelegatedSigners POSTs a signed getDelegatedSigners envelope.
func (c *Client) GetDelegatedSigners(ctx context.Context, req *types.SubAccountActionRequest) (types.DelegatedSignersResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.DelegatedSignersResponse
	if err := c.postTrade(ctx, "getDelegatedSigners", req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetDelegationsForDelegate POSTs a signed
// getDelegationsForDelegate envelope.
func (c *Client) GetDelegationsForDelegate(ctx context.Context, req *types.SubAccountActionRequest) (types.DelegationsForDelegateResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.DelegationsForDelegateResponse
	if err := c.postTrade(ctx, "getDelegationsForDelegate", req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------------
// Subaccount lifecycle (signed writes)
// ---------------------------------------------------------------------

// CreateSubaccount POSTs a signed createSubaccount envelope.
func (c *Client) CreateSubaccount(ctx context.Context, req *types.CreateSubaccountRequest) (*types.CreateSubaccountResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	if req.Params.Action == "" {
		req.Params.Action = "createSubaccount"
	}
	var out types.CreateSubaccountResponse
	if err := c.postTrade(ctx, "createSubaccount", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateSubAccountName POSTs a signed updateSubAccountName envelope.
func (c *Client) UpdateSubAccountName(ctx context.Context, req *types.UpdateSubAccountNameRequest) (*types.UpdateSubAccountNameResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	if req.Params.Action == "" {
		req.Params.Action = "updateSubAccountName"
	}
	var out types.UpdateSubAccountNameResponse
	if err := c.postTrade(ctx, "updateSubAccountName", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AddDelegatedSigner POSTs a signed addDelegatedSigner envelope.
func (c *Client) AddDelegatedSigner(ctx context.Context, req *types.AddDelegatedSignerRequest) (*types.AddDelegatedSignerResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	if req.Params.Action == "" {
		req.Params.Action = "addDelegatedSigner"
	}
	var out types.AddDelegatedSignerResponse
	if err := c.postTrade(ctx, "addDelegatedSigner", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RemoveDelegatedSigner POSTs a signed removeDelegatedSigner envelope.
func (c *Client) RemoveDelegatedSigner(ctx context.Context, req *types.RemoveDelegatedSignerRequest) (*types.RemoveDelegatedSignerResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	var out types.RemoveDelegatedSignerResponse
	if err := c.postTrade(ctx, "removeDelegatedSigner", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RemoveAllDelegatedSigners POSTs a signed removeAllDelegatedSigners
// envelope.
func (c *Client) RemoveAllDelegatedSigners(ctx context.Context, req *types.RemoveAllDelegatedSignersRequest) (*types.RemoveAllDelegatedSignersResponse, error) {
	if req == nil {
		return nil, errors.New("resttrade: request is required")
	}
	if req.Params.Action == "" {
		req.Params.Action = "removeAllDelegatedSigners"
	}
	var out types.RemoveAllDelegatedSignersResponse
	if err := c.postTrade(ctx, "removeAllDelegatedSigners", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------------------------------------------------------------
// Escape hatch for already-serialized envelopes
// ---------------------------------------------------------------------

// SendSigned is the low-level passthrough for callers that construct
// the signed envelope themselves. Body is the raw JSON envelope; out is
// the destination for the APIResponse.response field (nil to ignore).
//
// action is the action name (e.g. "placeOrders"); only used for error
// messages, not wire-format handling.
//
// NOTE: callers MUST ensure body is a well-formed signed envelope.
// resttrade does not inspect or mutate it.
func (c *Client) SendSigned(ctx context.Context, action string, body json.RawMessage, out any) error {
	if len(body) == 0 {
		return errors.New("resttrade: body is required")
	}
	return c.postTradeRaw(ctx, action, body, out)
}

// ---------------------------------------------------------------------
// Internal plumbing
// ---------------------------------------------------------------------

// postTrade marshals req to JSON and delegates to postTradeRaw.
func (c *Client) postTrade(ctx context.Context, action string, req any, out any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("resttrade: marshal %s request: %w", action, err)
	}
	return c.postTradeRaw(ctx, action, body, out)
}

// postTradeRaw is the single point every signed write goes through.
// Mirrors restinfo.postInfo's error taxonomy: transport errors map
// to *TransportError, structured API error branches map to *RESTError.
func (c *Client) postTradeRaw(ctx context.Context, action string, body json.RawMessage, out any) error {
	url := c.baseURL + "/v1/trade"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("resttrade: build %s request: %w", action, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
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

	var envelope types.RawAPIResponse
	if decodeErr := json.Unmarshal(respBody, &envelope); decodeErr != nil {
		return &TransportError{
			Action:       action,
			StatusCode:   resp.StatusCode,
			Err:          fmt.Errorf("decode envelope: %w", decodeErr),
			RawBodyBytes: respBody,
		}
	}

	if envelope.Error != nil {
		return &RESTError{
			Action:     action,
			StatusCode: resp.StatusCode,
			RequestID:  envelope.RequestID,
			Code:       envelope.Error.Code,
			Message:    envelope.Error.Message,
			Details:    envelope.Error.Details,
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

	if out == nil {
		return nil
	}
	if len(envelope.Response) == 0 || bytes.Equal(envelope.Response, []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(envelope.Response, out); err != nil {
		return fmt.Errorf("resttrade: decode %s response: %w", action, err)
	}
	return nil
}

// ---------------------------------------------------------------------
// Errors (shape-matched with restinfo for consistency across packages)
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
		return fmt.Sprintf("resttrade: transport error on %s (status %d): %v", e.Action, e.StatusCode, e.Err)
	}
	return fmt.Sprintf("resttrade: transport error on %s: %v", e.Action, e.Err)
}

func (e *TransportError) Unwrap() error { return e.Err }

// RESTError is returned when the API replies with a structured
// error branch.
type RESTError struct {
	Action     string
	StatusCode int
	RequestID  string
	Code       string
	Message    string
	Details    json.RawMessage
}

func (e *RESTError) Error() string {
	return fmt.Sprintf("resttrade: %s returned API error %s (status %d): %s", e.Action, e.Code, e.StatusCode, e.Message)
}
