// This file extends the types package with the shapes used by the
// /v1/ws/info endpoint: request/response envelopes, notification
// envelope, and per-channel payload types for public streams.
//
// Public and authenticated WebSocket streams are modelled separately so
// callers can choose the appropriate client.
//
// Wire contract follows the public WebSocket API.

package types

import "encoding/json"

// ---------------------------------------------------------------------
// Client -> server request envelope
// ---------------------------------------------------------------------

// WSRequest is the envelope every client request on /v1/ws/info uses:
// {"id": ..., "method": ..., "params": ...}.
type WSRequest struct {
	ID     string         `json:"id"`
	Method string         `json:"method"` // "subscribe" | "unsubscribe" | "ping"
	Params map[string]any `json:"params,omitempty"`
}

// ---------------------------------------------------------------------
// Server -> client response / notification envelope
// ---------------------------------------------------------------------

// WSMessage is the union envelope for both request-response replies
// and push notifications. Readers must branch on whether ID / Status
// are set (reply) vs Channel / Method set (push).
//
// Raw is kept so callers that need the original bytes don't have to
// re-marshal.
type WSMessage struct {
	// Request-response fields. Set when this is a reply to a
	// subscribe/unsubscribe/ping.
	ID        string          `json:"id,omitempty"`
	RequestID string          `json:"requestId,omitempty"`
	Status    int             `json:"status,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *WSError        `json:"error,omitempty"`

	// Notification fields. Set when this is a server push.
	Method    string          `json:"method,omitempty"`
	Channel   string          `json:"channel,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Timestamp int64           `json:"timestamp,omitempty"`

	// Sequencing / provenance. Optional; presence depends on channel.
	Seq       uint64 `json:"seq,omitempty"`
	PrevSeq   uint64 `json:"prevSeq,omitempty"`
	Meseq     uint64 `json:"meseq,omitempty"`
	PrevMeseq uint64 `json:"prevMeseq,omitempty"`
	Met       int64  `json:"met,omitempty"`
	Checksum  string `json:"checksum,omitempty"`

	// NeedsResync is present on account-stream payloads after an
	// upstream skip; not observed on /v1/ws/info today but kept
	// additive-safe.
	NeedsResync bool `json:"needsResync,omitempty"`

	// Raw is the untouched JSON bytes of this message. Populated by
	// wsinfo on read; unused on marshal.
	Raw json.RawMessage `json:"-"`
}

// IsNotification reports whether this message is a server push rather
// than a request-response reply.
func (m *WSMessage) IsNotification() bool {
	return m.ID == "" && m.RequestID == "" && (m.Method != "" || m.Channel != "")
}

// WSError is the structured error branch on a WSMessage reply.
type WSError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ---------------------------------------------------------------------
// Subscription params (per channel)
// ---------------------------------------------------------------------

// WSChannelTrade, WSChannelOrderbookUpdate, etc. are the canonical
// channel identifiers returned on notification envelopes. These are
// the values callers match against WSMessage.Channel.
const (
	WSChannelTrade           = "trade"
	WSChannelOrderbookUpdate = "orderbookUpdate"
	WSChannelPriceUpdate     = "priceUpdate"
	WSChannelCandleUpdate    = "candleUpdate"
	WSChannelLiquidation     = "liquidation"
)

// WSSubscribeType is the value of the "type" field in a subscribe
// params object. These are the request-side identifiers, which may
// differ from the notification Channel value (e.g. "orderbook" /
// "orderbookUpdate").
const (
	WSSubscribeTrade       = "trade"
	WSSubscribeOrderbook   = "orderbook"
	WSSubscribePrice       = "price"
	WSSubscribeCandle      = "candle"
	WSSubscribeLiquidation = "liquidations"
)

// ---------------------------------------------------------------------
// Per-channel typed payloads
// ---------------------------------------------------------------------
//
// These mirror the public streaming payloads. Fields are
// additive-safe: a new field on the wire is simply ignored by callers
// who haven't updated.

// WSTradeEvent is the data field of a "trade" notification.
type WSTradeEvent struct {
	Symbol        string `json:"symbol"`
	TradeID       string `json:"tradeId,omitempty"`
	Price         string `json:"price"`
	Quantity      string `json:"quantity"`
	Side          string `json:"side"` // "buy" | "sell"
	TimestampMs   int64  `json:"tsMs,omitempty"`
	IsTaker       bool   `json:"isTaker,omitempty"`
	BuyerOrderID  string `json:"buyerOrderId,omitempty"`
	SellerOrderID string `json:"sellerOrderId,omitempty"`
}

// WSPriceUpdateEvent is the data field of a "priceUpdate"
// notification. Fields mirror the REST getMarketPrices ticker shape.
type WSPriceUpdateEvent struct {
	Symbol         string `json:"symbol"`
	MarkPrice      string `json:"markPrice,omitempty"`
	IndexPrice     string `json:"indexPrice,omitempty"`
	LastPrice      string `json:"lastPrice,omitempty"`
	BidPrice       string `json:"bid,omitempty"`
	AskPrice       string `json:"ask,omitempty"`
	High24h        string `json:"high24h,omitempty"`
	Low24h         string `json:"low24h,omitempty"`
	Volume24h      string `json:"volume24h,omitempty"`
	PriceChange24h string `json:"priceChange24h,omitempty"`
	TimestampMs    int64  `json:"tsMs,omitempty"`
}

// WSCandleEvent is the data field of a "candleUpdate" notification.
type WSCandleEvent struct {
	Symbol    string `json:"symbol"`
	Timeframe string `json:"timeframe"`
	OpenTime  int64  `json:"openTime"`
	CloseTime int64  `json:"closeTime"`
	Open      string `json:"open"`
	High      string `json:"high"`
	Low       string `json:"low"`
	Close     string `json:"close"`
	Volume    string `json:"volume"`
	Closed    bool   `json:"closed,omitempty"`
}

// WSOrderbookEvent is the data field of an "orderbookUpdate"
// notification. Type distinguishes "snapshot" vs "diff"; in diff
// mode, Bids/Asks contain incremental updates (price, quantity=0 means
// remove).
type WSOrderbookEvent struct {
	Symbol      string       `json:"symbol"`
	Type        string       `json:"type"` // "snapshot" | "diff"
	Bids        []PriceLevel `json:"bids,omitempty"`
	Asks        []PriceLevel `json:"asks,omitempty"`
	Depth       int          `json:"depth,omitempty"`
	TimestampMs int64        `json:"tsMs,omitempty"`
}

// WSLiquidationEvent is the data field of a "liquidation"
// notification on /v1/ws/info (public, no account fields).
type WSLiquidationEvent struct {
	EventID          string `json:"eventId"`
	Market           string `json:"market"`
	TimestampMs      int64  `json:"tsMs"`
	Side             string `json:"side"` // "buy" | "sell"
	Size             string `json:"size"`
	LiquidationPrice string `json:"liquidationPrice"`
}
