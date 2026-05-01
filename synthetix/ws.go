package synthetix

import (
	"context"

	"github.com/synthetixio/synthetix-go/types"
	"github.com/synthetixio/synthetix-go/wsinfo"
)

// WSHandler receives public websocket messages.
type WSHandler = wsinfo.Handler

// WSOption configures simple public websocket subscriptions.
type WSOption func(*wsinfo.SubscribeSpec)

func WithDepth(depth int) WSOption {
	return func(s *wsinfo.SubscribeSpec) { s.Depth = depth }
}

func WithUpdateFrequencyMs(ms int) WSOption {
	return func(s *wsinfo.SubscribeSpec) { s.UpdateFrequencyMs = ms }
}

func WithSnapshot() WSOption {
	return func(s *wsinfo.SubscribeSpec) { s.Format = "snapshot" }
}

func WithDiffs() WSOption {
	return func(s *wsinfo.SubscribeSpec) { s.Format = "diff" }
}

func WithCandleInterval(interval string) WSOption {
	return func(s *wsinfo.SubscribeSpec) { s.Timeframe = interval }
}

// SubscribePublic subscribes to any public websocket channel.
func (c *Client) SubscribePublic(ctx context.Context, typ, symbol string, h WSHandler, opts ...WSOption) (func(), error) {
	spec := wsinfo.SubscribeSpec{Type: typ, Symbol: symbol}
	for _, opt := range opts {
		if opt != nil {
			opt(&spec)
		}
	}
	return c.wsInfo.Subscribe(ctx, spec, h)
}

func (c *Client) SubscribeTrades(ctx context.Context, symbol string, h WSHandler) (func(), error) {
	return c.SubscribePublic(ctx, types.WSSubscribeTrade, symbol, h)
}

func (c *Client) SubscribeOrderbook(ctx context.Context, symbol string, h WSHandler, opts ...WSOption) (func(), error) {
	return c.SubscribePublic(ctx, types.WSSubscribeOrderbook, symbol, h, opts...)
}

func (c *Client) SubscribePrices(ctx context.Context, symbol string, h WSHandler) (func(), error) {
	return c.SubscribePublic(ctx, types.WSSubscribePrice, symbol, h)
}

func (c *Client) SubscribeCandles(ctx context.Context, symbol, interval string, h WSHandler) (func(), error) {
	return c.SubscribePublic(ctx, types.WSSubscribeCandle, symbol, h, WithCandleInterval(interval))
}

func (c *Client) SubscribeLiquidations(ctx context.Context, symbol string, h WSHandler) (func(), error) {
	return c.SubscribePublic(ctx, types.WSSubscribeLiquidation, symbol, h)
}

// StreamPublic is the channel-oriented form of SubscribePublic.
func (c *Client) StreamPublic(ctx context.Context, typ, symbol string, buffer int, opts ...WSOption) (<-chan *types.WSMessage, func(), error) {
	if buffer <= 0 {
		buffer = 1
	}
	ch := make(chan *types.WSMessage, buffer)
	unsubscribe, err := c.SubscribePublic(ctx, typ, symbol, func(msg *types.WSMessage) {
		select {
		case ch <- msg:
		default:
			<-ch
			ch <- msg
		}
	}, opts...)
	if err != nil {
		close(ch)
		return nil, nil, err
	}
	return ch, func() {
		unsubscribe()
		close(ch)
	}, nil
}

func (c *Client) StreamTrades(ctx context.Context, symbol string, buffer int) (<-chan *types.WSMessage, func(), error) {
	return c.StreamPublic(ctx, types.WSSubscribeTrade, symbol, buffer)
}

func (c *Client) StreamOrderbook(ctx context.Context, symbol string, buffer int, opts ...WSOption) (<-chan *types.WSMessage, func(), error) {
	return c.StreamPublic(ctx, types.WSSubscribeOrderbook, symbol, buffer, opts...)
}
