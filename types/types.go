// Package types contains REST-native Go structs that mirror the JSON
// wire format of the public Synthetix V4 REST API (/v1/info, /v1/trade).
//
// The package is deliberately standalone and dependency-light so any
// Go program can pull it in to talk to api.synthetix.io.
//
// Field names and JSON tags here must match the public API response
// shapes byte-for-byte.
package types

import (
	"bytes"
	"encoding/json"
)

// APIResponse is the generic envelope returned by every /v1/info and
// /v1/trade call. Exactly one of Response and Error is non-nil on a
// well-formed response.
//
// The public API returns this shape for REST responses. The SDK keeps a
// local copy so the package stays dependency-light.
type APIResponse[T any] struct {
	RequestID string    `json:"requestId"`
	Response  T         `json:"response,omitempty"`
	Error     *APIError `json:"error,omitempty"`
}

// APIError is the error branch of APIResponse.
type APIError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details,omitempty"`
}

// RawAPIResponse is used when the caller wants to inspect the
// response envelope without deserializing Response into a concrete
// type. Useful in tests and for pass-through logging.
type RawAPIResponse = APIResponse[json.RawMessage]

// ---------------------------------------------------------------------
// Exchange status (getExchangeStatus)
// Public status payload.
// ---------------------------------------------------------------------

// ExchangeStatusResponse is the /v1/info getExchangeStatus payload.
type ExchangeStatusResponse struct {
	AcceptingOrders bool   `json:"accepting_orders"`
	ExchangeStatus  string `json:"exchange_status"` // "RUNNING" | "MAINTENANCE"
	Code            string `json:"code,omitempty"`  // "SERVICE_DRAINING" | "STATUS_DEGRADED" | ""
	Message         string `json:"message"`
	TimestampMs     int64  `json:"timestamp_ms"`
}

// IsRunning reports whether the exchange is currently accepting
// orders. Convenience wrapper over AcceptingOrders so callers don't
// have to reason about the RUNNING/MAINTENANCE string literals.
func (s ExchangeStatusResponse) IsRunning() bool { return s.AcceptingOrders }

// IsDegraded reports whether the status endpoint returned a degraded
// signal. A degraded response is still RUNNING; callers should surface
// it as an advisory hint rather than a hard stop.
func (s ExchangeStatusResponse) IsDegraded() bool { return s.Code == "STATUS_DEGRADED" }

// ---------------------------------------------------------------------
// Markets (getMarkets / getMarket helper)
// Market metadata.
// ---------------------------------------------------------------------

// MarketResponse is one market's full configuration. The getMarkets
// action returns []MarketResponse; GetMarket(symbol) filters it
// client-side.
type MarketResponse struct {
	Symbol      string `json:"symbol"`      // e.g. "SOL-USDT"
	Description string `json:"description"` // e.g. "Solana"
	BaseAsset   string `json:"baseAsset"`   // e.g. "SOL"
	QuoteAsset  string `json:"quoteAsset"`  // e.g. "USDT"

	IsOpen                     bool   `json:"isOpen"`
	IsCloseOnly                bool   `json:"isCloseOnly"`
	PriceExponent              int64  `json:"priceExponent"`
	QuantityExponent           int64  `json:"quantityExponent"`
	PriceIncrement             string `json:"priceIncrement"`
	MinOrderSize               string `json:"minOrderSize"`
	OrderSizeIncrement         string `json:"orderSizeIncrement"`
	ContractSize               uint32 `json:"contractSize"`
	MaxMarketOrderSize         string `json:"maxMarketOrderSize"`
	MaxLimitOrderSize          string `json:"maxLimitOrderSize"`
	MinOrderPrice              string `json:"minOrderPrice"`
	LimitOrderPriceCapRatio    string `json:"limitOrderPriceCapRatio"`
	LimitOrderPriceFloorRatio  string `json:"limitOrderPriceFloorRatio"`
	MarketOrderPriceCapRatio   string `json:"marketOrderPriceCapRatio"`
	MarketOrderPriceFloorRatio string `json:"marketOrderPriceFloorRatio"`
	LiquidationClearanceFee    string `json:"liquidationClearanceFee"`
	MinNotionalValue           string `json:"minNotionalValue"`

	MaintenanceMarginTiers []MaintenanceMarginTier `json:"maintenanceMarginTiers"`
}

// MaintenanceMarginTier is one entry in a market's maintenance margin
// tier schedule.
type MaintenanceMarginTier struct {
	LowerBound                   string `json:"lowerBound,omitempty"`
	UpperBound                   string `json:"upperBound,omitempty"`
	MaintenanceMarginFraction    string `json:"maintenanceMarginFraction,omitempty"`
	InitialMarginFraction        string `json:"initialMarginFraction,omitempty"`
	MinPositionSize              string `json:"minPositionSize,omitempty"`
	MaxPositionSize              string `json:"maxPositionSize,omitempty"`
	InitialMarginRequirement     string `json:"initialMarginRequirement,omitempty"`
	MaintenanceMarginRequirement string `json:"maintenanceMarginRequirement,omitempty"`
	MaintenanceDeductionValue    string `json:"maintenanceDeductionValue,omitempty"`
	MaxLeverage                  string `json:"maxLeverage"`
}

// UnmarshalJSON accepts maxLeverage as either a JSON string or number.
func (m *MaintenanceMarginTier) UnmarshalJSON(data []byte) error {
	type alias MaintenanceMarginTier
	var raw struct {
		alias
		MaxLeverage json.RawMessage `json:"maxLeverage"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = MaintenanceMarginTier(raw.alias)
	m.MaxLeverage = stringFromJSONScalar(raw.MaxLeverage)
	return nil
}

func stringFromJSONScalar(data json.RawMessage) string {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return ""
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		return s
	}
	return string(data)
}

// ---------------------------------------------------------------------
// Market prices (getMarketPrices)
// Market prices.
// ---------------------------------------------------------------------

// MarketPriceResponse is one symbol's current ticker. The API
// getMarketPrices action returns a map[symbol]MarketPriceResponse.
//
// The 24h ticker fields include volume, quote volume, open interest,
// and previous-day price.
type MarketPriceResponse struct {
	Symbol         string `json:"symbol"`
	BestBid        string `json:"bestBid"`
	BestAsk        string `json:"bestAsk"`
	MarkPrice      string `json:"markPrice"`
	IndexPrice     string `json:"indexPrice"`
	LastPrice      string `json:"lastPrice"`
	PrevDayPrice   string `json:"prevDayPrice"`
	Volume24h      string `json:"volume24h"`
	QuoteVolume24h string `json:"quoteVolume24h"`
	FundingRate    string `json:"fundingRate"`
	OpenInterest   string `json:"openInterest"`
	Timestamp      int64  `json:"timestamp"`
}

// ---------------------------------------------------------------------
// Orderbook (getOrderbook)
// Orderbook payload.
// ---------------------------------------------------------------------

// OrderbookResponse is the /v1/info getOrderbook payload. Bids and
// Asks are two-element tuples [price, quantity] encoded as a JSON
// array of arrays; we model them explicitly to make call sites
// type-safe.
type OrderbookResponse struct {
	Symbol    string          `json:"symbol"`
	Bids      []PriceLevel    `json:"bids"`
	Asks      []PriceLevel    `json:"asks"`
	Timestamp int64           `json:"timestamp,omitempty"`
	Sequence  int64           `json:"sequence,omitempty"`
	Checksum  json.RawMessage `json:"checksum,omitempty"` // Optional; present on WS diffs
}

// PriceLevel is one [price, quantity] tuple from an orderbook payload.
// We implement custom JSON to match the wire format which is a
// two-element JSON array rather than an object.
type PriceLevel struct {
	Price    string
	Quantity string
}

// MarshalJSON serializes PriceLevel as the wire-format two-element
// array ["price", "quantity"].
func (p PriceLevel) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]string{p.Price, p.Quantity})
}

// UnmarshalJSON accepts either the wire-format two-element array
// ["price", "quantity"] or an object form {price, quantity}. The array
// form is what the API returns; the object form is accepted for
// resilience against future shape changes.
func (p *PriceLevel) UnmarshalJSON(data []byte) error {
	var arr [2]string
	if err := json.Unmarshal(data, &arr); err == nil {
		p.Price = arr[0]
		p.Quantity = arr[1]
		return nil
	}
	var obj struct {
		Price    string `json:"price"`
		Quantity string `json:"quantity"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	p.Price = obj.Price
	p.Quantity = obj.Quantity
	return nil
}

// ---------------------------------------------------------------------
// Candles (getCandles)
// Candle payload.
// ---------------------------------------------------------------------

// CandleResponse is the /v1/info getCandles payload.
type CandleResponse struct {
	Symbol   string   `json:"symbol"`
	Interval string   `json:"interval"`
	Candles  []Candle `json:"candles"`
}

// Candle is one candlestick bar.
type Candle struct {
	OpenTime    int64  `json:"openTime"`
	CloseTime   int64  `json:"closeTime"`
	OpenPrice   string `json:"openPrice"`
	HighPrice   string `json:"highPrice"`
	LowPrice    string `json:"lowPrice"`
	ClosePrice  string `json:"closePrice"`
	Volume      string `json:"volume"`
	QuoteVolume string `json:"quoteVolume"`
	TradeCount  int32  `json:"tradeCount"`
}

// ---------------------------------------------------------------------
// Last trades (getLastTrades)
// Last-trade payload.
// ---------------------------------------------------------------------

// LastTradesResponse is the /v1/info getLastTrades payload.
type LastTradesResponse struct {
	Symbol string  `json:"symbol"`
	Trades []Trade `json:"trades"`
}

// Trade is one public trade from the tape.
type Trade struct {
	TradeID     string `json:"tradeId"`
	Symbol      string `json:"symbol"`
	Price       string `json:"price"`
	Quantity    string `json:"quantity"`
	Side        string `json:"side"` // "BUY" | "SELL" (aggressor side)
	TimestampMs int64  `json:"timestampMs"`
}

// ---------------------------------------------------------------------
// Funding rate (getFundingRate)
// Funding-rate payload.
// ---------------------------------------------------------------------

// FundingRateResponse is the /v1/info getFundingRate payload.
//
// FundingRateResponse is the current funding-rate payload.
type FundingRateResponse struct {
	Symbol               string `json:"symbol"`
	EstimatedFundingRate string `json:"estimatedFundingRate"`
	LastSettlementRate   string `json:"lastSettlementRate"`
	LastSettlementTime   int64  `json:"lastSettlementTime,omitempty"`
	NextFundingTime      int64  `json:"nextFundingTime,omitempty"`
	FundingInterval      int64  `json:"fundingInterval"`
}

// FundingRateHistoryResponse is the /v1/info getFundingRateHistory
// payload.
type FundingRateHistoryResponse struct {
	Symbol  string               `json:"symbol,omitempty"`
	History []FundingRateHistory `json:"history"`
}

// FundingRateHistory is one historical funding-rate observation.
type FundingRateHistory struct {
	Symbol      string `json:"symbol,omitempty"`
	FundingRate string `json:"fundingRate"`
	Timestamp   int64  `json:"timestamp"`
}

// ---------------------------------------------------------------------
// Open interest (getOpenInterest)
// Open-interest payload.
// ---------------------------------------------------------------------

// OpenInterestEntry is one symbol's open interest snapshot. The
// /v1/info getOpenInterest action returns []OpenInterestEntry.
//
// OpenInterestEntry is one open-interest row.
type OpenInterestEntry struct {
	Symbol            string `json:"symbol"`
	OpenInterest      string `json:"openInterest"`
	LongOpenInterest  string `json:"longOpenInterest,omitempty"`
	ShortOpenInterest string `json:"shortOpenInterest,omitempty"`
	Timestamp         int64  `json:"timestamp,omitempty"`
}

// ---------------------------------------------------------------------
// Mids (getMids)
// ---------------------------------------------------------------------

// MidResponse is one symbol's mid price. getMids returns a map keyed
// by symbol; we expose both the map entry shape (MidResponse) and the
// response as a map for callers that want the whole snapshot.
type MidResponse struct {
	Symbol    string `json:"symbol"`
	MidPrice  string `json:"midPrice"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

// ---------------------------------------------------------------------
// Subaccount ids (getSubAccountIds)
// Subaccount-id payload.
// ---------------------------------------------------------------------

// SubAccountIdsResponse is the /v1/info getSubAccountIds payload when
// includeDelegations is false. In that shape the response is a flat
// JSON array of string ids; api-client.ts types this as `string[]`.
//
// We wrap the array in a struct only so the restinfo client's return
// type stays consistent with the other payloads. The JSON unmarshals
// directly into the embedded slice via a custom UnmarshalJSON below.
type SubAccountIdsResponse struct {
	IDs []string
}

// MarshalJSON serializes as a bare JSON array to match the wire format.
func (s SubAccountIdsResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.IDs)
}

// UnmarshalJSON accepts a bare JSON array of ids.
func (s *SubAccountIdsResponse) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &s.IDs)
}

// SubAccountIdsWithDelegationsResponse is the /v1/info getSubAccountIds
// payload when includeDelegations is true. The shape is an object
// with both owned and delegated id lists.
type SubAccountIdsWithDelegationsResponse struct {
	SubAccountIDs          []string `json:"subAccountIds"`
	DelegatedSubAccountIDs []string `json:"delegatedSubAccountIds"`
}
