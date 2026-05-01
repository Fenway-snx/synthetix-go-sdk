// Package wsinfo is a shared upstream client for the public
// Synthetix V4 /v1/ws/info WebSocket endpoint.
//
// Design invariants:
//
//  1. One upstream *gorilla/websocket* connection per Client. Every
//     local subscriber in the same process shares that connection.
//  2. Reference-counted upstream subscriptions: if N local callers
//     subscribe to the same (channel, symbol, depth) tuple, wsinfo
//     issues exactly one upstream subscribe; the upstream unsubscribe
//     happens only when the last local caller drops.
//  3. Callback API (no channels): Subscribe returns an unsubscribe
//     func plus an error. Fan-out is drop-oldest with a bounded
//     per-subscriber ring buffer — slow consumers see gaps but never
//     block the read loop.
//  4. Public streams only. No auth message, no subAccountUpdate.
//     That surface belongs to a future /v1/ws/trade client.
//  5. Orderbook uses format=diff and forwards messages verbatim. No
//     checksum / sequence validation in v1; callers may opt in later.
package wsinfo

import (
	"fmt"
	"sort"
	"strings"
)

// SubscribeSpec describes what a local caller wants to subscribe to.
// It intentionally omits the wire-format "method"/"id" — wsinfo owns
// those. Callers supply only the semantic intent.
type SubscribeSpec struct {
	// Type is the request-side channel identifier sent in params.type.
	// Use the WSSubscribe* constants in the types package:
	//   types.WSSubscribeTrade, types.WSSubscribeOrderbook,
	//   types.WSSubscribePrice, types.WSSubscribeCandle,
	//   types.WSSubscribeLiquidation.
	Type string

	// Symbol is the canonical market symbol (e.g. "BTC-USDT"). Required
	// for every public channel on /v1/ws/info.
	Symbol string

	// Depth applies only to orderbook subscriptions. Valid values are
	// 10, 50, 100. Zero means the API default.
	Depth int

	// Timeframe applies only to candle subscriptions (e.g. "1m", "5m").
	Timeframe string

	// UpdateFrequencyMs applies only to orderbook subscriptions. Valid
	// values are 50, 100, 250, 500, 1000. Zero means the API default.
	UpdateFrequencyMs int

	// Format applies only to orderbook subscriptions. "diff" (default)
	// or "snapshot". Left empty means the API default.
	Format string
}

// validate returns a non-nil error if the spec is obviously
// malformed. It does not second-guess the API; unknown Types fail at
// subscribe time with a WSError, which is the right place for that
// feedback.
func (s SubscribeSpec) validate() error {
	if strings.TrimSpace(s.Type) == "" {
		return fmt.Errorf("wsinfo: subscribe spec Type is required")
	}
	if strings.TrimSpace(s.Symbol) == "" {
		return fmt.Errorf("wsinfo: subscribe spec Symbol is required")
	}
	return nil
}

// key returns the canonical string key used to deduplicate upstream
// subscriptions. Two specs that produce the same key share exactly
// one upstream subscribe message and exactly one notification fan-out
// slot.
//
// The key construction is stable across process restarts and across
// map iteration orderings: we sort the individual param components
// before joining so that reordering struct-field assignments in
// callers cannot produce a different key.
func (s SubscribeSpec) key() string {
	parts := []string{
		"type=" + s.Type,
		"symbol=" + strings.ToUpper(strings.TrimSpace(s.Symbol)),
	}
	if s.Depth > 0 {
		parts = append(parts, fmt.Sprintf("depth=%d", s.Depth))
	}
	if s.Timeframe != "" {
		parts = append(parts, "tf="+s.Timeframe)
	}
	if s.UpdateFrequencyMs > 0 {
		parts = append(parts, fmt.Sprintf("freq=%d", s.UpdateFrequencyMs))
	}
	if s.Format != "" {
		parts = append(parts, "fmt="+s.Format)
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

// toWireParams converts the spec to the params map sent on the wire in
// a subscribe request. It applies API defaults implicitly by omitting
// zero-valued fields.
func (s SubscribeSpec) toWireParams() map[string]any {
	p := map[string]any{
		"type":   s.Type,
		"symbol": strings.ToUpper(strings.TrimSpace(s.Symbol)),
	}
	if s.Depth > 0 {
		p["depth"] = s.Depth
	}
	if s.Timeframe != "" {
		p["timeframe"] = s.Timeframe
	}
	if s.UpdateFrequencyMs > 0 {
		p["updateFrequencyMs"] = s.UpdateFrequencyMs
	}
	if s.Format != "" {
		p["format"] = s.Format
	}
	return p
}

// notificationChannel returns the expected value of WSMessage.Channel
// for notifications produced by a subscribe of this spec's Type. It
// is the reverse of the Type -> Channel mapping used by the public API.
//
// Returns the empty string for unknown Types, which tells the
// fan-out layer to match by Type directly (defensive fallback for
// future channels).
func (s SubscribeSpec) notificationChannel() string {
	switch s.Type {
	case "orderbook":
		return "orderbookUpdate"
	case "price":
		return "priceUpdate"
	case "candle":
		return "candleUpdate"
	case "trade":
		return "trade"
	case "liquidations":
		return "liquidation"
	default:
		return ""
	}
}
