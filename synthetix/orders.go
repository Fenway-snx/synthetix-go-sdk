package synthetix

import (
	"context"
	"errors"
	"fmt"

	"github.com/synthetixio/synthetix-go/signer"
	"github.com/synthetixio/synthetix-go/types"
)

const (
	SideBuy  = "buy"
	SideSell = "sell"
)

const (
	// OrderTypeLimit keeps the historical SDK default for callers that
	// pass explicit time-in-force separately.
	OrderTypeLimit = "LIMIT"

	OrderTypeLimitGTC  = "limitGtc"
	OrderTypeLimitIOC  = "limitIoc"
	OrderTypeLimitGTD  = "limitGtd"
	OrderTypeMarket    = "market"
	OrderTypeTriggerTP = "triggerTp"
	OrderTypeTriggerSL = "triggerSl"
	OrderTypeTWAP      = "twap"
)

const (
	GroupingNone         = "na"
	GroupingPositionTPSL = "positionTpsl"
	GroupingTWAP         = "twap"
)

const (
	TimeInForceGTC = "GTC"
	TimeInForceIOC = "IOC"
	TimeInForceFOK = "FOK"
)

// GTC marks a limit order good-till-cancelled.
func GTC() Option { return WithTimeInForce(TimeInForceGTC) }

// IOC marks a limit order immediate-or-cancel.
func IOC() Option { return WithTimeInForce(TimeInForceIOC) }

// FOK marks a limit order fill-or-kill.
func FOK() Option { return WithTimeInForce(TimeInForceFOK) }

func normalizeOrderSide(side string) string {
	switch {
	case stringsEqualFold(side, "BUY"):
		return SideBuy
	case stringsEqualFold(side, "SELL"):
		return SideSell
	default:
		return side
	}
}

func normalizeOrderType(orderType, timeInForce string) string {
	if !stringsEqualFold(orderType, OrderTypeLimit) {
		return orderType
	}
	switch {
	case stringsEqualFold(timeInForce, TimeInForceIOC):
		return OrderTypeLimitIOC
	case stringsEqualFold(timeInForce, TimeInForceGTC), timeInForce == "":
		return OrderTypeLimitGTC
	default:
		return orderType
	}
}

func normalizePlaceOrderInput(order signer.PlaceOrderInput) signer.PlaceOrderInput {
	order.Side = normalizeOrderSide(order.Side)
	order.OrderType = normalizeOrderType(order.OrderType, order.TimeInForce)
	return order
}

func normalizePlaceOrderInputs(orders []signer.PlaceOrderInput) []signer.PlaceOrderInput {
	out := make([]signer.PlaceOrderInput, len(orders))
	for i := range orders {
		out[i] = normalizePlaceOrderInput(orders[i])
	}
	return out
}

// LimitGTCOrder submits a resting limit order.
func (c *Client) LimitGTCOrder(ctx context.Context, symbol, side, price, quantity string, opts ...Option) (*types.PlaceOrdersResponse, error) {
	return c.PlaceOrder(ctx, symbol, side, OrderTypeLimitGTC, price, quantity, opts...)
}

// LimitIOCOrder submits an immediate-or-cancel limit order.
func (c *Client) LimitIOCOrder(ctx context.Context, symbol, side, price, quantity string, opts ...Option) (*types.PlaceOrdersResponse, error) {
	return c.PlaceOrder(ctx, symbol, side, OrderTypeLimitIOC, price, quantity, opts...)
}

// LimitGTDOrder submits a good-till-date limit order.
func (c *Client) LimitGTDOrder(ctx context.Context, symbol, side, price, quantity string, expiresAt int64, opts ...Option) (*types.PlaceOrdersResponse, error) {
	return c.PlaceOrder(ctx, symbol, side, OrderTypeLimitGTD, price, quantity, append(opts, WithExpiresAt(expiresAt))...)
}

// TriggerTPOrder submits a take-profit trigger order.
func (c *Client) TriggerTPOrder(ctx context.Context, symbol, side, price, triggerPrice, quantity string, opts ...Option) (*types.PlaceOrdersResponse, error) {
	return c.PlaceOrder(ctx, symbol, side, OrderTypeTriggerTP, price, quantity, append(opts, WithGrouping(GroupingPositionTPSL), WithTriggerPrice(triggerPrice))...)
}

// TriggerSLOrder submits a stop-loss trigger order.
func (c *Client) TriggerSLOrder(ctx context.Context, symbol, side, price, triggerPrice, quantity string, opts ...Option) (*types.PlaceOrdersResponse, error) {
	return c.PlaceOrder(ctx, symbol, side, OrderTypeTriggerSL, price, quantity, append(opts, WithGrouping(GroupingPositionTPSL), WithTriggerPrice(triggerPrice))...)
}

// MarketOpen is a trader-friendly alias for MarketOrder.
func (c *Client) MarketOpen(ctx context.Context, symbol, side, quantity string, opts ...Option) (*types.PlaceOrdersResponse, error) {
	return c.MarketOrder(ctx, symbol, side, quantity, opts...)
}

// MarketClose closes the current position for symbol with a reduce-only
// market order.
func (c *Client) MarketClose(ctx context.Context, symbol string, opts ...Option) (*types.PlaceOrdersResponse, error) {
	pos, err := c.positionForSymbol(ctx, symbol, opts...)
	if err != nil {
		return nil, err
	}
	if pos.Quantity == "" {
		return nil, fmt.Errorf("synthetix: position %q has no quantity", symbol)
	}
	side, err := closeSide(pos.Side)
	if err != nil {
		return nil, err
	}
	return c.MarketOrder(ctx, symbol, side, pos.Quantity, append(opts, WithReduceOnly())...)
}

func (c *Client) positionForSymbol(ctx context.Context, symbol string, opts ...Option) (*types.Position, error) {
	positions, err := c.GetPositions(ctx, opts...)
	if err != nil {
		return nil, err
	}
	for i := range positions {
		if stringsEqualFold(positions[i].Symbol, symbol) {
			return &positions[i], nil
		}
	}
	return nil, fmt.Errorf("synthetix: no open position for %q", symbol)
}

func closeSide(side string) (string, error) {
	switch {
	case stringsEqualFold(side, SideBuy), stringsEqualFold(side, "LONG"):
		return SideSell, nil
	case stringsEqualFold(side, SideSell), stringsEqualFold(side, "SHORT"):
		return SideBuy, nil
	default:
		return "", errors.New("synthetix: position side must be buy/sell or long/short")
	}
}
