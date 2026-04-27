// This file extends the types package with the shapes used by the
// /v1/trade endpoint: signed-envelope request types and typed
// response bodies for signed actions.
//
// The resttrade client is a pure HTTP transport. It accepts
// *already-signed* request envelopes produced by the SDK signer or by
// an external signer. resttrade never holds or uses private key
// material.
//
// Consequently, every TradeXxxRequest type in this file is the
// envelope as it appears on the wire, with Signature already
// populated. Signing helpers live in the signer and eip712 packages;
// this package intentionally knows nothing about private keys.

package types

import "encoding/json"

// ---------------------------------------------------------------------
// Signature envelope common to every /v1/trade write
// ---------------------------------------------------------------------

// SignatureComponents is the EIP-712 (v, r, s) triple carried on
// every signed /v1/trade request. The wire format uses lowercase
// field names matching the TS SDK; see sample/node-scripts/src/types.ts.
//
// V is int rather than uint because the API accepts both the
// raw 0..3 recovery id and the legacy 27..30 encoding.
type SignatureComponents struct {
	V int    `json:"v"`
	R string `json:"r"` // hex, "0x..."
	S string `json:"s"` // hex, "0x..."
}

// ---------------------------------------------------------------------
// Place orders
// ---------------------------------------------------------------------

// PlaceOrderItem is one order inside a legacy PlaceOrdersAction
// helper. Kept for backwards compatibility with external SDK
// snippets.
type PlaceOrderItem struct {
	Symbol          string `json:"symbol"`
	Side            string `json:"side"`
	OrderType       string `json:"orderType"`
	Price           string `json:"price"`
	TriggerPrice    string `json:"triggerPrice,omitempty"`
	Quantity        string `json:"quantity"`
	ReduceOnly      bool   `json:"reduceOnly,omitempty"`
	PostOnly        bool   `json:"postOnly,omitempty"`
	IsTriggerMarket bool   `json:"isTriggerMarket,omitempty"`
	ClientOrderID   string `json:"clientOrderId,omitempty"`
	ClosePosition   bool   `json:"closePosition,omitempty"`
	ExpiresAt       int64  `json:"expiresAt,omitempty"`
	TimeInForce     string `json:"timeInForce,omitempty"`
	DurationSeconds int64  `json:"durationSeconds,omitempty"`
	IntervalSeconds int64  `json:"intervalSeconds,omitempty"`
}

// PlaceOrdersAction is a legacy typed helper for the /v1/trade
// placeOrders params body. Prefer Signer helpers or pass the exact
// signed action payload through PlaceOrdersRequest.Params instead.
type PlaceOrdersAction struct {
	Action       string           `json:"action"`
	SubAccountID string           `json:"subaccountId"`
	Orders       []PlaceOrderItem `json:"orders"`
	Grouping     string           `json:"grouping,omitempty"`
}

// PlaceOrdersRequest is the full signed envelope POSTed to /v1/trade.
//
// The wire format is the signed trade envelope: subaccountId,
// walletAddress, signature, nonce, and expiresAfter live at the top
// level, with the full action payload living under params. Params is
// intentionally `any` so callers can keep the signed bytes
// byte-identical to what goes on the wire.
type PlaceOrdersRequest struct {
	Params        any                 `json:"params"`
	Signature     SignatureComponents `json:"signature"`
	SubAccountID  string              `json:"subaccountId"`
	WalletAddress string              `json:"walletAddress"`
	Nonce         uint64              `json:"nonce"`
	ExpiresAfter  int64               `json:"expiresAfter,omitempty"`
}

// OrderStatus is the per-order outcome inside every order-outcome
// wrapper the /v1/trade write surface returns (placeOrders,
// cancelOrders, compound orders, TPSL, and TWAP orders).
//
//   - Resting / Filled / Canceled carry an OrderIdentifier (under
//     `order`) AND a deprecated flat VenueID (under `id`), plus
//     ExpiresAt on Resting/Filled.
//   - Error / ErrorCode / ErrorOrderId populate together on a
//     failed order.
//
// Exactly one of Resting, Filled, Canceled, Error (+ ErrorCode +
// ErrorOrderId) is populated per entry; the wire format does not
// guarantee tagged-union discipline so callers should treat the
// whole thing as a flat bag and prefer the first non-nil/non-empty
// branch in presentation order.
type OrderStatus struct {
	Resting      *OrderRestingStatus  `json:"resting,omitempty"`
	Filled       *OrderFilledStatus   `json:"filled,omitempty"`
	Canceled     *OrderCanceledStatus `json:"canceled,omitempty"`
	Error        string               `json:"error,omitempty"`
	ErrorCode    string               `json:"errorCode,omitempty"`
	ErrorOrderId *ErrorOrderIDResponse `json:"order,omitempty"`
}

type OrderRestingStatus struct {
	OrderID           OrderIdentifier `json:"order"`
	DeprecatedVenueID string          `json:"id"`
	ExpiresAt         *int64          `json:"expiresAt,omitempty"`
}

type OrderFilledStatus struct {
	OrderID           OrderIdentifier `json:"order"`
	DeprecatedVenueID string          `json:"id"`
	TotalSize         string          `json:"totalSize"`
	AvgPrice          string          `json:"avgPrice"`
	ExpiresAt         *int64          `json:"expiresAt,omitempty"`
}

type OrderCanceledStatus struct {
	OrderID           OrderIdentifier `json:"order"`
	DeprecatedVenueID string          `json:"id"`
}

// ErrorOrderIDResponse is surfaced under `order` on an errored status
// so callers can tell which submitted order the error pertains to. The
// deprecated VenueID string is the legacy flat venue identifier.
type ErrorOrderIDResponse struct {
	Order             OrderIdentifier `json:"order"`
	DeprecatedVenueID string          `json:"id,omitempty"`
}

// PlaceOrdersResponse is the response body returned on a successful
// /v1/trade placeOrders. The wire shape is { statuses: [...] }.
type PlaceOrdersResponse struct {
	Statuses []OrderStatus `json:"statuses"`
}

// ---------------------------------------------------------------------
// Modify order
// ---------------------------------------------------------------------

// ModifyOrderAction is a legacy typed helper retained for external SDK
// snippets. Prefer Signer helpers for new code.
type ModifyOrderAction struct {
	Action        string `json:"action"`
	SubAccountID  string `json:"subaccountId"`
	OrderID       string `json:"orderId,omitempty"`
	ClientOrderID string `json:"clientOrderId,omitempty"`
	Price         string `json:"price,omitempty"`
	Quantity      string `json:"quantity,omitempty"`
}

// ModifyOrderRequest is the full signed envelope. Params is the
// caller-supplied action payload so the signed bytes match the posted
// bytes exactly.
type ModifyOrderRequest struct {
	Params        any                 `json:"params"`
	Signature     SignatureComponents `json:"signature"`
	SubAccountID  string              `json:"subaccountId"`
	WalletAddress string              `json:"walletAddress"`
	Nonce         uint64              `json:"nonce"`
	ExpiresAfter  int64               `json:"expiresAfter,omitempty"`
}

// ModifyOrderResponse is the modifyOrder response. On success the API
// emits an OrderIdentifier under `order` plus a deprecated flat
// `orderId`. Error / ErrorCode populate together on a failed modify;
// Timestamp is the response timestamp in milliseconds.
type ModifyOrderResponse struct {
	Order              OrderIdentifier `json:"order"`
	DeprecatedVenueID  string          `json:"orderId,omitempty"`
	Status             string          `json:"status,omitempty"`
	Price              string          `json:"price,omitempty"`
	Quantity           string          `json:"quantity,omitempty"`
	TriggerPrice       string          `json:"triggerPrice,omitempty"`
	CumQty             string          `json:"cumQty,omitempty"`
	AvgPrice           string          `json:"avgPrice,omitempty"`
	Error              string          `json:"error,omitempty"`
	ErrorCode          string          `json:"errorCode,omitempty"`
	Timestamp          int64           `json:"timestamp"`
}

// ---------------------------------------------------------------------
// Cancel orders / cancel all
// ---------------------------------------------------------------------

// CancelOrdersAction is a legacy typed helper retained for external SDK
// snippets. Prefer Signer helpers for new code.
type CancelOrdersAction struct {
	Action         string   `json:"action"`
	SubAccountID   string   `json:"subaccountId"`
	OrderIDs       []string `json:"orderIds,omitempty"`
	ClientOrderIDs []string `json:"clientOrderIds,omitempty"`
}

// CancelOrdersRequest is the full signed envelope. Params is the
// caller-supplied action payload.
type CancelOrdersRequest struct {
	Params        any                 `json:"params"`
	Signature     SignatureComponents `json:"signature"`
	SubAccountID  string              `json:"subaccountId"`
	WalletAddress string              `json:"walletAddress"`
	Nonce         uint64              `json:"nonce"`
	ExpiresAfter  int64               `json:"expiresAfter,omitempty"`
}

// CancelOrdersResponse uses the same { statuses: [...] } envelope used
// by placeOrders. Per-entry shape is OrderStatus, with Canceled
// populated for successful cancels and Error / ErrorCode / ErrorOrderId
// for failures.
type CancelOrdersResponse struct {
	Statuses []OrderStatus `json:"statuses"`
}

// CancelAllOrdersAction is a legacy typed helper retained for external
// SDK snippets. Prefer Signer helpers for new code.
type CancelAllOrdersAction struct {
	Action       string   `json:"action"`
	SubAccountID string   `json:"subaccountId"`
	Symbols      []string `json:"symbols,omitempty"`
}

// CancelAllOrdersRequest is the full signed envelope.
type CancelAllOrdersRequest struct {
	Params        any                 `json:"params"`
	Signature     SignatureComponents `json:"signature"`
	SubAccountID  string              `json:"subaccountId"`
	WalletAddress string              `json:"walletAddress"`
	Nonce         uint64              `json:"nonce"`
	ExpiresAfter  int64               `json:"expiresAfter,omitempty"`
}

// CancelAllOrdersResponse is a *bare array* of per-order result
// entries, not a wrapped { count } object. Each entry carries the
// canonical OrderIdentifier under `order`, the deprecated flat
// venue id under `orderId`, a human-readable `message`
// (populated on error; empty on success), and an optional `symbol`
// pointer identifying the market the cancel applied to.
type CancelAllOrdersResponse []CancelAllOrderResult

// CancelAllOrderResult is one entry in CancelAllOrdersResponse.
// Symbol is a pointer to preserve the wire-level distinction
// between "cancelled order had no symbol context" (nil) and
// "cancelled order's symbol was empty string" (&"").
type CancelAllOrderResult struct {
	Order             OrderIdentifier `json:"order"`
	DeprecatedVenueID string          `json:"orderId,omitempty"`
	Message           string          `json:"message"`
	Symbol            *string         `json:"symbol"`
}

// ---------------------------------------------------------------------
// Authenticated reads (GET-style signed actions)
// ---------------------------------------------------------------------

// SubAccountActionRequest is the generic signed-envelope for
// authenticated reads such as getSubAccount, getSubAccounts,
// getOpenOrders, getPositions, getFills. Params is optional; when
// set the API treats its "action" key as authoritative.
type SubAccountActionRequest struct {
	Params        map[string]any      `json:"params,omitempty"`
	Signature     SignatureComponents `json:"signature"`
	SubAccountID  string              `json:"subaccountId"`
	WalletAddress string              `json:"walletAddress"`
	Nonce         uint64              `json:"nonce,omitempty"`
	ExpiresAfter  int64               `json:"expiresAfter,omitempty"`
}

// SubAccountResponse mirrors the `/v1/trade` getSubAccount +
// getSubAccounts response body. It intentionally tracks the public wire
// shape: a nested crossMarginSummary, a feeRates block, collaterals /
// positions / marketPreferences / accountLimits maps. Fields are typed
// as string/int64 because this package is a pure wire mirror.
type SubAccountResponse struct {
	SubAccountID      string                `json:"subAccountId"`
	MasterAccountID   *string               `json:"masterAccountId"`
	Name              string                `json:"subAccountName"`
	Collaterals       []CollateralResponse  `json:"collaterals"`
	MarginSummary     MarginSummary         `json:"crossMarginSummary"`
	Positions         []SubAccountPosition  `json:"positions"`
	MarketPreferences MarketPreferences     `json:"marketPreferences"`
	FeeRates          FeeRateInfo           `json:"feeRates"`
	AccountLimits     AccountLimitsResponse `json:"accountLimits"`
}

// MarginSummary mirrors the `crossMarginSummary` block inside a
// SubAccountResponse. JSON tags must stay in lockstep with the public
// wire shape.
type MarginSummary struct {
	AccountValue         string `json:"accountValue"`
	AvailableMargin      string `json:"availableMargin"`
	UnrealizedPnl        string `json:"totalUnrealizedPnl"`
	MaintenanceMargin    string `json:"maintenanceMargin"`
	InitialMargin        string `json:"initialMargin"`
	Withdrawable         string `json:"withdrawable"`
	AdjustedAccountValue string `json:"adjustedAccountValue"`
	Debt                 string `json:"debt"`
}

// FeeRateInfo mirrors the `feeRates` block. Maker/Taker are wire-
// format decimal strings (e.g. "0.0002"); TierName is a human-
// readable label like "Regular User".
type FeeRateInfo struct {
	MakerFeeRate string `json:"makerFeeRate"`
	TakerFeeRate string `json:"takerFeeRate"`
	TierName     string `json:"tierName"`
}

// CollateralResponse mirrors one entry inside
// SubAccountResponse.Collaterals. HaircutRate / HaircutAdjustment are
// optional in the wire format (older API builds omit them),
// so keep them `omitempty` on the mirror to stay permissive.
type CollateralResponse struct {
	AdjustedCollateralValue string `json:"adjustedCollateralValue"`
	CollateralValue         string `json:"collateralValue"`
	HaircutRate             string `json:"haircutRate,omitempty"`
	HaircutAdjustment       string `json:"haircutAdjustment,omitempty"`
	PendingWithdraw         string `json:"pendingWithdraw"`
	Price                   string `json:"price"`
	CalculatedAt            int64  `json:"calculatedAt"`
	Quantity                string `json:"quantity"`
	Symbol                  string `json:"symbol"`
	Withdrawable            string `json:"withdrawable"`
}

// SubAccountPosition mirrors one entry inside
// SubAccountResponse.Positions. Distinct from the top-level Position
// struct (returned by getPositions) because the nested shape carries
// a reduced field set — specifically it has `pnl` + `upnl` strings
// where getPositions has `unrealizedPnl` / `realizedPnl`. Keeping
// them separate avoids papering over a real wire-format split.
type SubAccountPosition struct {
	Symbol            string `json:"symbol"`
	Side              string `json:"side"` // "long" | "short"
	EntryPrice        string `json:"entryPrice"`
	Quantity          string `json:"quantity"`
	Pnl               string `json:"pnl"`
	Upnl              string `json:"upnl"`
	UsedMargin        string `json:"usedMargin"`
	MaintenanceMargin string `json:"maintenanceMargin"`
	LiquidationPrice  string `json:"liquidationPrice"`
}

// MarketPreferences mirrors the per-subaccount leverage overrides
// surfaced on the getSubAccount response.
type MarketPreferences struct {
	Leverages map[string]uint32 `json:"leverages"`
}

// AccountLimitsResponse mirrors the `accountLimits` block.
type AccountLimitsResponse struct {
	MaxBorrowCapacity  string `json:"maxBorrowCapacity"`
	MaxOrdersPerMarket int64  `json:"maxOrdersPerMarket"`
	MaxSubAccounts     int64  `json:"maxSubAccounts"`
	MaxTotalOrders     int64  `json:"maxTotalOrders"`
}

// SubAccountWithDelegatesResponse is SubAccountResponse plus the
// list of delegated signers attached to the subaccount. Wire field
// name is `delegatedSigners`, NOT `delegates`.
type SubAccountWithDelegatesResponse struct {
	SubAccountResponse
	DelegatedSigners []DelegatedSignerInfo `json:"delegatedSigners"`
}

// DelegatedSignerInfo mirrors one entry inside
// SubAccountWithDelegatesResponse.DelegatedSigners. Includes
// SubAccountID so callers can demultiplex a delegate that covers
// multiple subaccounts, and AddedBy for audit when available.
type DelegatedSignerInfo struct {
	SubAccountID  string   `json:"subAccountId"`
	WalletAddress string   `json:"walletAddress"`
	Permissions  []string  `json:"permissions"`
	ExpiresAt    *int64    `json:"expiresAt"`
	AddedBy      *string   `json:"addedBy,omitempty"`
}

// OrderIdentifier is the canonical composite order identifier used
// across the /v1/trade surface. VenueID is a decimal string so full
// uint64 precision survives JSON transport.
type OrderIdentifier struct {
	VenueID  string `json:"venueId"`
	ClientID string `json:"clientId,omitempty"`
}

// TWAPDetails mirrors the optional TWAP execution block on an
// OpenOrder. Only populated when Type=="twap".
type TWAPDetails struct {
	AveragePrice    string `json:"averagePrice"`
	IntervalMs      int64  `json:"intervalMs"`
	TotalTrades     int    `json:"totalTrades"`
	TradesFilled    int    `json:"tradesFilled"`
	TotalFees       string `json:"totalFees"`
	StartedAtMs     int64  `json:"startedAtMs"`
	TotalDurationMs int64  `json:"totalDurationMs"`
}

// OpenOrder is one resting order returned by getOpenOrders. The wire
// shape uses `order`/`takeProfitOrder`/`stopLossOrder` objects and
// deprecated flat `orderId`/`takeProfitOrderId`/`stopLossOrderId`
// venue ids. There is NO `remainingQty` / `status` / `orderType` on
// the wire; callers synthesize those from the fields below if needed.
type OpenOrder struct {
	Order                OrderIdentifier  `json:"order"`
	DeprecatedVenueID    string           `json:"orderId,omitempty"`
	Symbol               string           `json:"symbol"`
	Side                 string           `json:"side"`
	Type                 string           `json:"type"`
	Quantity             string           `json:"quantity"`
	Price                string           `json:"price"`
	TriggerPrice         string           `json:"triggerPrice,omitempty"`
	TriggerPriceType     string           `json:"triggerPriceType,omitempty"`
	TimeInForce          string           `json:"timeInForce,omitempty"`
	ReduceOnly           bool             `json:"reduceOnly,omitempty"`
	PostOnly             bool             `json:"postOnly,omitempty"`
	CreatedTime          int64            `json:"createdTime,omitempty"`
	UpdatedTime          int64            `json:"updatedTime,omitempty"`
	FilledQuantity       string           `json:"filledQuantity,omitempty"`
	TakeProfitOrder      *OrderIdentifier `json:"takeProfitOrder,omitempty"`
	DeprecatedTakeProfit string           `json:"takeProfitOrderId,omitempty"`
	StopLossOrder        *OrderIdentifier `json:"stopLossOrder,omitempty"`
	DeprecatedStopLoss   string           `json:"stopLossOrderId,omitempty"`
	ClosePosition        bool             `json:"closePosition,omitempty"`
	ExpiresAt            *int64           `json:"expiresAt,omitempty"`
	TwapDetails          *TWAPDetails     `json:"twapDetails,omitempty"`
}

// Position is one open position returned by getPositions. It uses the
// top-level shape: RealizedPnl / UnrealizedPnl, and the canonical
// `takeProfitOrders`/`stopLossOrders` OrderIdentifier lists alongside
// the deprecated flat `takeProfitOrderIds`/`stopLossOrderIds` venue
// ids.
type Position struct {
	ADLBucket                  int64             `json:"adlBucket"`
	PositionID                 string            `json:"positionId"`
	SubAccountID               string            `json:"subAccountId"`
	Symbol                     string            `json:"symbol"`
	Side                       string            `json:"side"`
	EntryPrice                 string            `json:"entryPrice,omitempty"`
	Quantity                   string            `json:"quantity,omitempty"`
	RealizedPnl                string            `json:"realizedPnl,omitempty"`
	UnrealizedPnl              string            `json:"unrealizedPnl,omitempty"`
	UsedMargin                 string            `json:"usedMargin,omitempty"`
	MaintenanceMargin          string            `json:"maintenanceMargin,omitempty"`
	LiquidationPrice           string            `json:"liquidationPrice,omitempty"`
	Status                     string            `json:"status,omitempty"`
	NetFunding                 string            `json:"netFunding,omitempty"`
	TakeProfitOrders           []OrderIdentifier `json:"takeProfitOrders,omitempty"`
	DeprecatedTakeProfitOrders []string          `json:"takeProfitOrderIds,omitempty"`
	StopLossOrders             []OrderIdentifier `json:"stopLossOrders,omitempty"`
	DeprecatedStopLossOrders   []string          `json:"stopLossOrderIds,omitempty"`
	UpdatedAt                  int64             `json:"updatedAt,omitempty"`
	CreatedAt                  int64             `json:"createdAt,omitempty"`
}

// ---------------------------------------------------------------------
// Subaccount lifecycle actions
// ---------------------------------------------------------------------

// CreateSubaccountAction is the params payload for createSubaccount.
type CreateSubaccountAction struct {
	Action       string `json:"action"` // "createSubaccount"
	SubAccountID string `json:"subaccountId"`
	Name         string `json:"name"`
}

// CreateSubaccountRequest is the full signed envelope.
type CreateSubaccountRequest struct {
	Params       CreateSubaccountAction `json:"params"`
	Nonce        uint64                 `json:"nonce"`
	Signature    SignatureComponents    `json:"signature"`
	ExpiresAfter int64                  `json:"expiresAfter,omitempty"`
}

// CreateSubaccountResponse is the response body.
type CreateSubaccountResponse struct {
	SubAccountID   string `json:"subAccountId"`
	SubAccountName string `json:"subAccountName,omitempty"`
}

// UpdateSubAccountNameAction is the params payload for
// updateSubAccountName.
type UpdateSubAccountNameAction struct {
	Action string `json:"action"` // "updateSubAccountName"
	Name   string `json:"name"`
}

// UpdateSubAccountNameRequest is the full signed envelope.
type UpdateSubAccountNameRequest struct {
	Params       UpdateSubAccountNameAction `json:"params"`
	SubAccountID string                     `json:"subaccountId"`
	Nonce        uint64                     `json:"nonce"`
	Signature    SignatureComponents        `json:"signature"`
	ExpiresAfter int64                      `json:"expiresAfter,omitempty"`
}

// UpdateSubAccountNameResponse is the response body.
type UpdateSubAccountNameResponse struct {
	SubAccountID string `json:"subAccountId"`
	Name         string `json:"name"`
}

// ---------------------------------------------------------------------
// Delegated signer lifecycle
// ---------------------------------------------------------------------

// AddDelegatedSignerAction is the params payload for
// addDelegatedSigner.
type AddDelegatedSignerAction struct {
	Action        string   `json:"action"` // "addDelegatedSigner"
	WalletAddress string   `json:"walletAddress"`
	Permissions   []string `json:"permissions"`
	ExpiresAt     int64    `json:"expiresAt,omitempty"` // 0 = never
}

// AddDelegatedSignerRequest is the full signed envelope.
type AddDelegatedSignerRequest struct {
	Params       AddDelegatedSignerAction `json:"params"`
	SubAccountID string                   `json:"subaccountId"`
	Nonce        uint64                   `json:"nonce"`
	Signature    SignatureComponents      `json:"signature"`
	ExpiresAfter int64                    `json:"expiresAfter,omitempty"`
}

// AddDelegatedSignerResponse is the response body.
type AddDelegatedSignerResponse struct {
	SubAccountID  string `json:"subAccountId"`
	WalletAddress string `json:"walletAddress"`
}

// RemoveAllDelegatedSignersAction is the params payload for
// removeAllDelegatedSigners.
type RemoveAllDelegatedSignersAction struct {
	Action string `json:"action"` // "removeAllDelegatedSigners"
}

// RemoveAllDelegatedSignersRequest is the full signed envelope.
type RemoveAllDelegatedSignersRequest struct {
	Params       RemoveAllDelegatedSignersAction `json:"params"`
	SubAccountID string                          `json:"subaccountId"`
	Nonce        uint64                          `json:"nonce"`
	Signature    SignatureComponents             `json:"signature"`
	ExpiresAfter int64                           `json:"expiresAfter,omitempty"`
}

// RemoveAllDelegatedSignersResponse is the response body.
type RemoveAllDelegatedSignersResponse struct {
	SubAccountID   string   `json:"subAccountId"`
	RemovedSigners []string `json:"removedSigners"`
}

// ---------------------------------------------------------------------
// Authenticated history reads
// ---------------------------------------------------------------------

// OrderHistoryItem is one order-history entry. The wire shape uses an
// OrderIdentifier under `order` AND a
// deprecated flat decimal-string venue id under `orderId`; callers
// should prefer Order.VenueID and treat DeprecatedVenueID as a
// legacy fallback.
//
// Numeric fields are decimal strings (same convention as OpenOrder)
// so full precision survives JSON transport. Timestamps are
// milliseconds-since-epoch; `expiresAt` is a pointer because an
// absent `expiresAt` and an explicit zero have different meanings
// on the wire.
type OrderHistoryItem struct {
	Order                  OrderIdentifier `json:"order"`
	DeprecatedVenueID      string          `json:"orderId,omitempty"`
	Symbol                 string          `json:"symbol"`
	Side                   string          `json:"side"`
	Type                   string          `json:"type"`
	Quantity               string          `json:"quantity"`
	Price                  string          `json:"price"`
	Status                 string          `json:"status"`
	TimeInForce            string          `json:"timeInForce"`
	CreatedTime            int64           `json:"createdTime"`
	UpdateTime             int64           `json:"updateTime"`
	FilledQuantity         string          `json:"filledQuantity"`
	FilledPrice            string          `json:"filledPrice"`
	TriggeredByLiquidation bool            `json:"triggeredByLiquidation"`
	ReduceOnly             bool            `json:"reduceOnly"`
	PostOnly               bool            `json:"postOnly"`
	TriggerPrice           string          `json:"triggerPrice,omitempty"`
	TriggerPriceType       string          `json:"triggerPriceType,omitempty"`
	ExpiresAt              *int64          `json:"expiresAt,omitempty"`
	CancelReason           string          `json:"cancelReason,omitempty"`
}

// OrderHistoryResponse is a *bare array*, not a wrapped object. Keep
// this as a named slice type (not an alias) so callers can attach
// methods in the future without breaking the mirror contract.
type OrderHistoryResponse []OrderHistoryItem

// TradeHistoryItem is one trade-history entry. OrderType follows the
// canonical /v1/trade casing (e.g. "LIMIT"); Side is lowercased
// (`"buy"`/`"sell"`). `markPrice` and `entryPrice`
// are mark-price and position-avg-entry snapshots captured at trade
// time. `direction` is the legacy pre-conversion direction string
// (e.g. "LONG_OPEN") and is kept for callers who need raw venue
// semantics.
type TradeHistoryItem struct {
	TradeID                string          `json:"tradeId"`
	Order                  OrderIdentifier `json:"order"`
	DeprecatedVenueID      string          `json:"orderId,omitempty"`
	Symbol                 string          `json:"symbol"`
	OrderType              string          `json:"orderType"`
	Side                   string          `json:"side"`
	Price                  string          `json:"price"`
	Quantity               string          `json:"quantity"`
	RealizedPnl            string          `json:"realizedPnl"`
	Fee                    string          `json:"fee"`
	FeeRate                string          `json:"feeRate"`
	Timestamp              int64           `json:"timestamp"`
	Maker                  bool            `json:"maker"`
	ReduceOnly             bool            `json:"reduceOnly"`
	MarkPrice              string          `json:"markPrice"`
	EntryPrice             string          `json:"entryPrice"`
	TriggeredByLiquidation bool            `json:"triggeredByLiquidation"`
	Direction              string          `json:"direction"`
	PostOnly               bool            `json:"postOnly"`
}

// TradeHistoryResponse is a wrapped object with the trades array, `hasMore`
// flag, and an absolute `total` count across the filter (not just
// the current page). Pagination is caller-driven via offset+limit.
type TradeHistoryResponse struct {
	Trades  []TradeHistoryItem `json:"trades"`
	HasMore bool               `json:"hasMore"`
	Total   int                `json:"total"`
}

// FundingSummary contains aggregate funding totals. All fields are
// decimal strings; `TotalPayments` is a stringified integer count, not
// a dollar amount.
type FundingSummary struct {
	TotalFundingReceived string `json:"totalFundingReceived"`
	TotalFundingPaid     string `json:"totalFundingPaid"`
	NetFunding           string `json:"netFunding"`
	TotalPayments        string `json:"totalPayments"`
	AveragePaymentSize   string `json:"averagePaymentSize"`
}

// FundingPayment is one funding-payment entry. The API may emit both
// legacy timestamp fields and newer named timestamp fields, so the SDK
// carries both. `payment` is signed (negative = paid out).
type FundingPayment struct {
	PaymentID                                   string `json:"paymentId"`
	Symbol                                      string `json:"symbol"`
	PositionSize                                string `json:"positionSize"`
	FundingRate                                 string `json:"fundingRate"`
	Payment                                     string `json:"payment"`
	DeprecatedTimestampNowPaymentTime           int64  `json:"timestamp"`
	PaymentTime                                 int64  `json:"paymentTime"`
	DeprecatedFundingTimestampNowFundingTime    int64  `json:"fundingTimestamp"`
	FundingTime                                 int64  `json:"fundingTime"`
}

// FundingPaymentsResponse is the getFundingPayments response body.
type FundingPaymentsResponse struct {
	Summary        FundingSummary   `json:"summary"`
	FundingHistory []FundingPayment `json:"fundingHistory"`
}

// PerformanceHistoryPoint is one sampled performance-history point.
// `SampledAt` is milliseconds-since-epoch; `AccountValue` and `Pnl`
// are decimal strings preserving precision across the wire.
type PerformanceHistoryPoint struct {
	SampledAt    int64  `json:"sampledAt"`
	AccountValue string `json:"accountValue"`
	Pnl          string `json:"pnl"`
}

// PerformanceHistoryPeriod groups sampled performance history.
// Volume is the period-total notional traded (decimal string).
type PerformanceHistoryPeriod struct {
	History []PerformanceHistoryPoint `json:"history"`
	Volume  string                    `json:"volume"`
}

// PerformanceHistoryResponse is the getPerformanceHistory response
// body. Callers should trust the response subaccount id rather than
// echoing the requested subaccount.
type PerformanceHistoryResponse struct {
	SubAccountID string                   `json:"subAccountId"`
	Period       string                   `json:"period"`
	Performance  PerformanceHistoryPeriod `json:"performanceHistory"`
}

// ---------------------------------------------------------------------
// Additional account management write actions
// ---------------------------------------------------------------------

// UpdateLeverageRequest is the full signed envelope for
// updateLeverage.
type UpdateLeverageRequest struct {
	Params        map[string]any      `json:"params"`
	Signature     SignatureComponents `json:"signature"`
	SubAccountID  string              `json:"subaccountId"`
	WalletAddress string              `json:"walletAddress"`
	Nonce         uint64              `json:"nonce"`
	ExpiresAfter  int64               `json:"expiresAfter,omitempty"`
}

// UpdateLeverageResponse is intentionally permissive; deployments
// have returned both compact success objects and updated preference
// snapshots for this action.
type UpdateLeverageResponse map[string]any

// WithdrawCollateralRequest is the full signed envelope for
// withdrawCollateral.
type WithdrawCollateralRequest struct {
	Params        map[string]any      `json:"params"`
	Signature     SignatureComponents `json:"signature"`
	SubAccountID  string              `json:"subaccountId"`
	WalletAddress string              `json:"walletAddress"`
	Nonce         uint64              `json:"nonce"`
	ExpiresAfter  int64               `json:"expiresAfter,omitempty"`
}

// WithdrawCollateralResponse mirrors the commonly returned status
// fields while preserving additive fields via JSON decoding into the
// map-like Params on callers that need raw data.
type WithdrawCollateralResponse map[string]any

// TransferCollateralRequest is the full signed envelope for
// transferCollateral.
type TransferCollateralRequest struct {
	Params        map[string]any      `json:"params"`
	Signature     SignatureComponents `json:"signature"`
	SubAccountID  string              `json:"subaccountId"`
	WalletAddress string              `json:"walletAddress"`
	Nonce         uint64              `json:"nonce"`
	ExpiresAfter  int64               `json:"expiresAfter,omitempty"`
}

// TransferCollateralResponse is the transferCollateral response body.
type TransferCollateralResponse map[string]any

// ScheduleCancelRequest is the full signed envelope for scheduleCancel.
type ScheduleCancelRequest struct {
	Params        map[string]any      `json:"params"`
	Signature     SignatureComponents `json:"signature"`
	SubAccountID  string              `json:"subaccountId"`
	WalletAddress string              `json:"walletAddress"`
	Nonce         uint64              `json:"nonce"`
	ExpiresAfter  int64               `json:"expiresAfter,omitempty"`
}

// ScheduleCancelResponse is the scheduleCancel response body.
type ScheduleCancelResponse map[string]any

// RemoveDelegatedSignerRequest is the full signed envelope for
// removeDelegatedSigner.
type RemoveDelegatedSignerRequest struct {
	Params        map[string]any      `json:"params"`
	Signature     SignatureComponents `json:"signature"`
	SubAccountID  string              `json:"subaccountId"`
	WalletAddress string              `json:"walletAddress"`
	Nonce         uint64              `json:"nonce"`
	ExpiresAfter  int64               `json:"expiresAfter,omitempty"`
}

// RemoveDelegatedSignerResponse is the removeDelegatedSigner response
// body.
type RemoveDelegatedSignerResponse map[string]any

// ---------------------------------------------------------------------
// Additional authenticated reads
// ---------------------------------------------------------------------

// BalanceUpdate is one balance/collateral ledger update.
type BalanceUpdate struct {
	ID                 string `json:"id"`
	SubAccountID       string `json:"subAccountId,omitempty"`
	Action             string `json:"action"`
	Status             string `json:"status"`
	Amount             string `json:"amount"`
	GrossAmount        string `json:"grossAmount,omitempty"`
	Fee                string `json:"fee,omitempty"`
	Collateral         string `json:"collateral,omitempty"`
	Timestamp          int64  `json:"timestamp"`
	FromSubAccountID   string `json:"fromSubAccountId,omitempty"`
	ToSubAccountID     string `json:"toSubAccountId,omitempty"`
	DestinationAddress string `json:"destinationAddress,omitempty"`
	TxHash             string `json:"txHash,omitempty"`
}

// BalanceUpdatesResponse is the getBalanceUpdates response body.
type BalanceUpdatesResponse struct {
	BalanceUpdates []BalanceUpdate `json:"balanceUpdates"`
	HasMore        bool            `json:"hasMore,omitempty"`
	Total          int             `json:"total,omitempty"`
}

func (r *BalanceUpdatesResponse) UnmarshalJSON(data []byte) error {
	var wrapped struct {
		BalanceUpdates []BalanceUpdate `json:"balanceUpdates"`
		HasMore        bool            `json:"hasMore,omitempty"`
		Total          int             `json:"total,omitempty"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.BalanceUpdates != nil {
		*r = BalanceUpdatesResponse(wrapped)
		return nil
	}
	var bare []BalanceUpdate
	if err := json.Unmarshal(data, &bare); err != nil {
		return err
	}
	r.BalanceUpdates = bare
	return nil
}

// Transfer is one transfer history entry.
type Transfer struct {
	TransferID   string `json:"transferId"`
	From         string `json:"from"`
	To           string `json:"to"`
	Symbol       string `json:"symbol"`
	Amount       string `json:"amount"`
	TransferType string `json:"transferType"`
	Status       string `json:"status"`
	Timestamp    int64  `json:"timestamp"`
}

// TransfersResponse is the getTransfers response body.
type TransfersResponse struct {
	Transfers []Transfer `json:"transfers"`
	Total     int        `json:"total,omitempty"`
}

func (r *TransfersResponse) UnmarshalJSON(data []byte) error {
	var wrapped struct {
		Transfers []Transfer `json:"transfers"`
		Total     int        `json:"total,omitempty"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.Transfers != nil {
		*r = TransfersResponse(wrapped)
		return nil
	}
	var bare []Transfer
	if err := json.Unmarshal(data, &bare); err != nil {
		return err
	}
	r.Transfers = bare
	return nil
}

// PositionHistoryItem is one historical position entry.
type PositionHistoryItem struct {
	PositionID        string `json:"positionId"`
	SubAccountID      string `json:"subAccountId,omitempty"`
	Symbol            string `json:"symbol"`
	Side              string `json:"side"`
	EntryPrice        string `json:"entryPrice,omitempty"`
	ExitPrice         string `json:"exitPrice,omitempty"`
	Quantity          string `json:"quantity,omitempty"`
	RealizedPnl       string `json:"realizedPnl,omitempty"`
	UnrealizedPnl     string `json:"unrealizedPnl,omitempty"`
	OpenedAt          int64  `json:"openedAt,omitempty"`
	ClosedAt          int64  `json:"closedAt,omitempty"`
	CreatedAt         int64  `json:"createdAt,omitempty"`
	UpdatedAt         int64  `json:"updatedAt,omitempty"`
	Status            string `json:"status,omitempty"`
	NetFunding        string `json:"netFunding,omitempty"`
	LiquidationPrice  string `json:"liquidationPrice,omitempty"`
	MaintenanceMargin string `json:"maintenanceMargin,omitempty"`
}

// PositionHistoryResponse is the getPositionHistory response body.
type PositionHistoryResponse struct {
	Positions []PositionHistoryItem `json:"positions"`
	HasMore   bool                  `json:"hasMore,omitempty"`
	Total     int                   `json:"total,omitempty"`
}

func (r *PositionHistoryResponse) UnmarshalJSON(data []byte) error {
	var wrapped struct {
		Positions []PositionHistoryItem `json:"positions"`
		HasMore   bool                  `json:"hasMore,omitempty"`
		Total     int                   `json:"total,omitempty"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.Positions != nil {
		*r = PositionHistoryResponse(wrapped)
		return nil
	}
	var bare []PositionHistoryItem
	if err := json.Unmarshal(data, &bare); err != nil {
		return err
	}
	r.Positions = bare
	return nil
}

// PortfolioSnapshot is one account portfolio snapshot.
type PortfolioSnapshot struct {
	Timestamp    int64            `json:"timestamp,omitempty"`
	AccountValue string           `json:"accountValue,omitempty"`
	Pnl          string           `json:"pnl,omitempty"`
	Assets       []PortfolioAsset `json:"assets,omitempty"`
}

// PortfolioAsset is one asset entry in a portfolio snapshot.
type PortfolioAsset struct {
	Symbol string `json:"symbol"`
	Amount string `json:"amount,omitempty"`
	Value  string `json:"value,omitempty"`
}

// PortfolioResponse is the getPortfolio response body.
type PortfolioResponse []PortfolioSnapshot

func (r *PortfolioResponse) UnmarshalJSON(data []byte) error {
	var bare []PortfolioSnapshot
	if err := json.Unmarshal(data, &bare); err == nil {
		*r = bare
		return nil
	}
	var wrapped struct {
		Portfolio []PortfolioSnapshot `json:"portfolio"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.Portfolio != nil {
		*r = wrapped.Portfolio
		return nil
	}
	var single PortfolioSnapshot
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*r = []PortfolioSnapshot{single}
	return nil
}

// TradesForPositionResponse is the getTradesForPosition response body.
type TradesForPositionResponse TradeHistoryResponse

// FeeTier is one available fee tier.
type FeeTier struct {
	Name          string `json:"name,omitempty"`
	TierName      string `json:"tierName,omitempty"`
	MakerFeeRate  string `json:"makerFeeRate,omitempty"`
	TakerFeeRate  string `json:"takerFeeRate,omitempty"`
	VolumeLowerBound string `json:"volumeLowerBound,omitempty"`
	VolumeUpperBound string `json:"volumeUpperBound,omitempty"`
}

// UserFeeTier is the caller's current fee tier.
type UserFeeTier struct {
	Symbol      string `json:"symbol,omitempty"`
	FeeRate     string `json:"feeRate,omitempty"`
	MakerFeeRate string `json:"makerFeeRate,omitempty"`
	TakerFeeRate string `json:"takerFeeRate,omitempty"`
	TierName    string `json:"tierName,omitempty"`
}

// FeesResponse is the getFees response body.
type FeesResponse struct {
	FeeTiers        []FeeTier   `json:"feeTiers,omitempty"`
	UserDailyVolume string      `json:"userDailyVolume,omitempty"`
	UserFeeTier     UserFeeTier `json:"userFeeTier,omitempty"`
	MakerFeeRate    string      `json:"makerFeeRate,omitempty"`
	TakerFeeRate    string      `json:"takerFeeRate,omitempty"`
}

// RateLimitsResponse is the getRateLimits response body.
type RateLimitsResponse struct {
	RequestsUsed int `json:"requestsUsed,omitempty"`
	RequestsCap  int `json:"requestsCap,omitempty"`
	Remaining    int `json:"remaining,omitempty"`
	ResetTime    int64 `json:"resetTime,omitempty"`
}

// DelegatedSignersResponse is the getDelegatedSigners response body.
type DelegatedSignersResponse []DelegatedSignerInfo

// DelegationsForDelegateResponse is the getDelegationsForDelegate
// response body.
type DelegationsForDelegateResponse []DelegatedSignerInfo
