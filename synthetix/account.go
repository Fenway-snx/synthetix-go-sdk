package synthetix

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/shopspring/decimal"

	"github.com/synthetixio/synthetix-go/signer"
	"github.com/synthetixio/synthetix-go/types"
)

// Option configures high-level authenticated helpers.
type Option func(*options)

type options struct {
	subAccountID   uint64
	overWS         bool
	expiresAfterMs int64
	clientOrderID  string
	reduceOnly     bool
	postOnly       bool
	expiresAt      int64
	timeInForce    string
	grouping       string
	triggerPrice   string
	triggerMarket  bool
	closePosition  bool
	durationSec    int64
	intervalSec    int64
	slippage       decimal.Decimal
	extra          map[string]any
}

func (c *Client) options(opts ...Option) options {
	o := options{
		expiresAfterMs: c.expiresAfterMs,
		grouping:       GroupingNone,
		timeInForce:    TimeInForceGTC,
		slippage:       decimal.RequireFromString("0.05"),
		extra:          map[string]any{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}

// WithReduceOnly marks an order reduce-only.
func WithReduceOnly() Option { return func(o *options) { o.reduceOnly = true } }

// WithPostOnly marks an order post-only.
func WithPostOnly() Option { return func(o *options) { o.postOnly = true } }

// WithClientOrderID attaches a client order id.
func WithClientOrderID(id string) Option { return func(o *options) { o.clientOrderID = id } }

// WithExpiresAt sets the order-level expiry timestamp in milliseconds.
func WithExpiresAt(ms int64) Option { return func(o *options) { o.expiresAt = ms } }

// WithExpiresAfterMs sets the signed-message expiry window in
// milliseconds.
func WithExpiresAfterMs(ms int64) Option { return func(o *options) { o.expiresAfterMs = ms } }

// WithSubAccount overrides the configured/default subaccount id.
func WithSubAccount(id uint64) Option { return func(o *options) { o.subAccountID = id } }

// OverWS sends the signed action over /v1/ws/trade instead of REST.
func OverWS() Option { return func(o *options) { o.overWS = true } }

// WithTimeInForce sets the order time-in-force.
func WithTimeInForce(tif string) Option { return func(o *options) { o.timeInForce = tif } }

// WithGrouping sets the Synthetix compound-order grouping.
func WithGrouping(grouping string) Option { return func(o *options) { o.grouping = grouping } }

// WithTriggerPrice sets the trigger price on trigger orders.
func WithTriggerPrice(price string) Option { return func(o *options) { o.triggerPrice = price } }

// WithTriggerMarket marks a trigger order as a market trigger.
func WithTriggerMarket() Option { return func(o *options) { o.triggerMarket = true } }

// WithClosePosition marks an order as close-position.
func WithClosePosition() Option { return func(o *options) { o.closePosition = true } }

// WithDurationSeconds sets a TWAP order duration.
func WithDurationSeconds(seconds int64) Option { return func(o *options) { o.durationSec = seconds } }

// WithIntervalSeconds sets the optional TWAP slice interval.
func WithIntervalSeconds(seconds int64) Option { return func(o *options) { o.intervalSec = seconds } }

// WithSlippage sets market-order slippage as a decimal fraction.
func WithSlippage(slippage decimal.Decimal) Option { return func(o *options) { o.slippage = slippage } }

// WithParam adds an action-specific parameter to authenticated read
// requests.
func WithParam(key string, value any) Option {
	return func(o *options) {
		if o.extra == nil {
			o.extra = map[string]any{}
		}
		o.extra[key] = value
	}
}

// WithOrderHistoryFilter sets getOrderHistory filters.
func WithOrderHistoryFilter(startTimeMs, endTimeMs int64, limit int) Option {
	return func(o *options) {
		setTimeRange(o, startTimeMs, endTimeMs)
		setLimit(o, limit)
	}
}

// WithTradesOrderID filters getTrades by venue order id.
func WithTradesOrderID(orderID string) Option {
	return func(o *options) {
		if orderID != "" {
			o.extra["orderId"] = orderID
		}
	}
}

// WithTradesForPositionFilter sets getTradesForPosition filters.
func WithTradesForPositionFilter(positionID string, limit, offset int) Option {
	return func(o *options) {
		if positionID != "" {
			o.extra["positionId"] = positionID
		}
		setLimit(o, limit)
		setOffset(o, offset)
	}
}

// WithBalanceUpdatesFilter sets getBalanceUpdates filters.
func WithBalanceUpdatesFilter(startTimeMs, endTimeMs int64, actionFilter string, limit, offset int) Option {
	return func(o *options) {
		setTimeRange(o, startTimeMs, endTimeMs)
		if actionFilter != "" {
			o.extra["actionFilter"] = actionFilter
		}
		setLimit(o, limit)
		setOffset(o, offset)
	}
}

// WithPositionHistoryFilter sets getPositionHistory filters.
func WithPositionHistoryFilter(symbol string, startTimeMs, endTimeMs int64, limit, offset int) Option {
	return func(o *options) {
		if symbol != "" {
			o.extra["symbol"] = symbol
		}
		setTimeRange(o, startTimeMs, endTimeMs)
		setLimit(o, limit)
		setOffset(o, offset)
	}
}

// WithPerformancePeriod sets getPerformanceHistory period
// ("day", "week", "month", or "year").
func WithPerformancePeriod(period string) Option {
	return func(o *options) {
		if period != "" {
			o.extra["period"] = period
		}
	}
}

func setTimeRange(o *options, startTimeMs, endTimeMs int64) {
	if startTimeMs > 0 {
		o.extra["startTime"] = startTimeMs
	}
	if endTimeMs > 0 {
		o.extra["endTime"] = endTimeMs
	}
}

func setLimit(o *options, limit int) {
	if limit > 0 {
		o.extra["limit"] = limit
	}
}

func setOffset(o *options, offset int) {
	if offset > 0 {
		o.extra["offset"] = offset
	}
}

func (c *Client) requireSigner() (*signer.Signer, error) {
	if c.signer == nil {
		return nil, ErrNoSigner
	}
	return c.signer, nil
}

func (c *Client) resolveSubAccount(ctx context.Context, o options) (uint64, error) {
	if o.subAccountID != 0 {
		return o.subAccountID, nil
	}
	c.subAccountMu.Lock()
	defer c.subAccountMu.Unlock()
	if c.subAccountID != 0 {
		return c.subAccountID, nil
	}
	if !c.autoDiscoverSubAcct {
		return 0, errors.New("synthetix: subaccount id is required")
	}
	s, err := c.requireSigner()
	if err != nil {
		return 0, err
	}
	ids, err := c.info.GetSubAccountIds(ctx, s.WalletAddress())
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, errors.New("synthetix: no subaccounts found for wallet")
	}
	id, err := strconv.ParseUint(ids[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("synthetix: parse subaccount id %q: %w", ids[0], err)
	}
	c.subAccountID = id
	if err := c.ensureWSTrade(id); err != nil {
		return 0, err
	}
	return id, nil
}

func (c *Client) signedRead(ctx context.Context, action string, opts ...Option) (*types.SubAccountActionRequest, options, uint64, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, o, 0, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, o, 0, err
	}
	req, err := s.SignSubAccountAction(subID, action, 0, o.expiresAfterMs)
	if err != nil {
		return nil, o, 0, err
	}
	for k, v := range o.extra {
		req.Params[k] = v
	}
	return req, o, subID, nil
}

func (c *Client) postWS(ctx context.Context, subAccountID uint64, req any, out any) error {
	if subAccountID == 0 {
		subID, ok := c.DefaultSubAccountID()
		if !ok {
			return errors.New("synthetix: authenticated websocket requires a default subaccount")
		}
		subAccountID = subID
	}
	if err := c.ensureWSTrade(subAccountID); err != nil {
		return err
	}
	return c.wsTrade.Post(ctx, req, out)
}

// PlaceOrder signs and submits one order.
func (c *Client) PlaceOrder(ctx context.Context, symbol, side, orderType, price, quantity string, opts ...Option) (*types.PlaceOrdersResponse, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, err
	}
	req, err := s.SignPlaceOrders(subID, []signer.PlaceOrderInput{{
		Symbol:          symbol,
		Side:            normalizeOrderSide(side),
		OrderType:       normalizeOrderType(orderType, o.timeInForce),
		Price:           price,
		TriggerPrice:    o.triggerPrice,
		Quantity:        quantity,
		ReduceOnly:      o.reduceOnly,
		PostOnly:        o.postOnly,
		IsTriggerMarket: o.triggerMarket,
		ClientOrderID:   o.clientOrderID,
		ClosePosition:   o.closePosition,
		ExpiresAt:       o.expiresAt,
		TimeInForce:     o.timeInForce,
		DurationSeconds: o.durationSec,
		IntervalSeconds: o.intervalSec,
	}}, o.grouping, 0, o.expiresAfterMs)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.PlaceOrdersResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.PlaceOrders(ctx, req)
}

// PlaceOrders signs and submits multiple orders.
func (c *Client) PlaceOrders(ctx context.Context, orders []signer.PlaceOrderInput, opts ...Option) (*types.PlaceOrdersResponse, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, err
	}
	req, err := s.SignPlaceOrders(subID, normalizePlaceOrderInputs(orders), o.grouping, 0, o.expiresAfterMs)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.PlaceOrdersResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.PlaceOrders(ctx, req)
}

// MarketOrder submits a native market order.
func (c *Client) MarketOrder(ctx context.Context, symbol, side, quantity string, opts ...Option) (*types.PlaceOrdersResponse, error) {
	return c.PlaceOrder(ctx, symbol, side, OrderTypeMarket, "", quantity, opts...)
}

// TwapOrder submits a TWAP order. Pass WithIntervalSeconds to override
// the API default slice interval.
func (c *Client) TwapOrder(ctx context.Context, symbol, side, quantity string, durationSeconds int64, price string, opts ...Option) (*types.PlaceOrdersResponse, error) {
	return c.PlaceOrder(
		ctx,
		symbol,
		side,
		OrderTypeTWAP,
		price,
		quantity,
		append(opts, WithGrouping(GroupingTWAP), WithDurationSeconds(durationSeconds))...,
	)
}

// ModifyOrder signs and submits a modify by venue id.
func (c *Client) ModifyOrder(ctx context.Context, orderID uint64, price, quantity, triggerPrice string, opts ...Option) (*types.ModifyOrderResponse, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, err
	}
	req, err := s.SignModifyOrder(subID, orderID, price, quantity, triggerPrice, 0, o.expiresAfterMs)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.ModifyOrderResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.ModifyOrder(ctx, req)
}

// ModifyOrderByCloid signs and submits a modify by client order id.
func (c *Client) ModifyOrderByCloid(ctx context.Context, clientOrderID, price, quantity, triggerPrice string, opts ...Option) (*types.ModifyOrderResponse, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, err
	}
	req, err := s.SignModifyOrderByCloid(subID, clientOrderID, price, quantity, triggerPrice, 0, o.expiresAfterMs)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.ModifyOrderResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.ModifyOrder(ctx, req)
}

// CancelOrder cancels one venue order id.
func (c *Client) CancelOrder(ctx context.Context, orderID uint64, opts ...Option) (*types.CancelOrdersResponse, error) {
	return c.CancelOrders(ctx, []uint64{orderID}, opts...)
}

// CancelOrders cancels venue order ids.
func (c *Client) CancelOrders(ctx context.Context, orderIDs []uint64, opts ...Option) (*types.CancelOrdersResponse, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, err
	}
	req, err := s.SignCancelOrders(subID, orderIDs, 0, o.expiresAfterMs)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.CancelOrdersResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.CancelOrders(ctx, req)
}

// CancelOrderByCloid cancels one client order id.
func (c *Client) CancelOrderByCloid(ctx context.Context, clientOrderID string, opts ...Option) (*types.CancelOrdersResponse, error) {
	return c.CancelOrdersByCloid(ctx, []string{clientOrderID}, opts...)
}

// CancelOrdersByCloid cancels client order ids.
func (c *Client) CancelOrdersByCloid(ctx context.Context, clientOrderIDs []string, opts ...Option) (*types.CancelOrdersResponse, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, err
	}
	req, err := s.SignCancelOrdersByCloid(subID, clientOrderIDs, 0, o.expiresAfterMs)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.CancelOrdersResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.CancelOrdersByCloid(ctx, req)
}

// CancelAllOrders cancels all open orders, optionally filtered by symbols.
func (c *Client) CancelAllOrders(ctx context.Context, symbols []string, opts ...Option) (*types.CancelAllOrdersResponse, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, err
	}
	req, err := s.SignCancelAllOrders(subID, symbols, 0, o.expiresAfterMs)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.CancelAllOrdersResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.CancelAllOrders(ctx, req)
}

// ScheduleCancel sets or clears the dead-man switch.
func (c *Client) ScheduleCancel(ctx context.Context, timeoutSeconds int64, opts ...Option) (*types.ScheduleCancelResponse, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, err
	}
	req, err := s.SignScheduleCancel(subID, timeoutSeconds, 0, o.expiresAfterMs)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.ScheduleCancelResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.ScheduleCancel(ctx, req)
}

// SlippagePrice returns a slippage-adjusted mid price for market-order helpers.
func (c *Client) SlippagePrice(ctx context.Context, symbol string, isBuy bool, slippage decimal.Decimal) (string, error) {
	prices, err := c.info.GetMarketPrices(ctx)
	if err != nil {
		return "", err
	}
	ticker, ok := prices[symbol]
	if !ok {
		return "", fmt.Errorf("synthetix: market price %q not found", symbol)
	}
	mid, err := decimal.NewFromString(ticker.MarkPrice)
	if err != nil {
		return "", err
	}
	mult := decimal.NewFromInt(1)
	if isBuy {
		mult = mult.Add(slippage)
	} else {
		mult = mult.Sub(slippage)
	}
	return mid.Mul(mult).String(), nil
}

// GetSubAccount returns the selected subaccount snapshot.
func (c *Client) GetSubAccount(ctx context.Context, opts ...Option) (*types.SubAccountResponse, error) {
	req, o, subID, err := c.signedRead(ctx, "getSubAccount", opts...)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.SubAccountResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetSubAccount(ctx, req)
}

// GetSubAccounts returns all subaccounts available to the signer.
func (c *Client) GetSubAccounts(ctx context.Context, opts ...Option) ([]types.SubAccountWithDelegatesResponse, error) {
	req, o, subID, err := c.signedRead(ctx, "getSubAccounts", opts...)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out []types.SubAccountWithDelegatesResponse
		return out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetSubAccounts(ctx, req)
}

func (c *Client) GetPositions(ctx context.Context, opts ...Option) ([]types.Position, error) {
	req, o, subID, err := c.signedRead(ctx, "getPositions", opts...)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out []types.Position
		return out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetPositions(ctx, req)
}

func (c *Client) GetOpenOrders(ctx context.Context, opts ...Option) ([]types.OpenOrder, error) {
	req, o, subID, err := c.signedRead(ctx, "getOpenOrders", opts...)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out []types.OpenOrder
		return out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetOpenOrders(ctx, req)
}

func (c *Client) GetOrderHistory(ctx context.Context, opts ...Option) (types.OrderHistoryResponse, error) {
	req, o, subID, err := c.signedRead(ctx, "getOrderHistory", opts...)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.OrderHistoryResponse
		return out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetOrderHistory(ctx, req)
}

func (c *Client) GetTrades(ctx context.Context, opts ...Option) (*types.TradeHistoryResponse, error) {
	req, o, subID, err := c.signedRead(ctx, "getTrades", opts...)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.TradeHistoryResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetTrades(ctx, req)
}

func (c *Client) GetTradesForPosition(ctx context.Context, opts ...Option) (*types.TradesForPositionResponse, error) {
	req, o, subID, err := c.signedRead(ctx, "getTradesForPosition", opts...)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.TradesForPositionResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetTradesForPosition(ctx, req)
}

func (c *Client) GetPortfolio(ctx context.Context, opts ...Option) (*types.PortfolioResponse, error) {
	req, o, subID, err := c.signedRead(ctx, "getPortfolio", opts...)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.PortfolioResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetPortfolio(ctx, req)
}

func (c *Client) GetBalanceUpdates(ctx context.Context, opts ...Option) (types.BalanceUpdatesResponse, error) {
	req, o, subID, err := c.signedRead(ctx, "getBalanceUpdates", opts...)
	if err != nil {
		return types.BalanceUpdatesResponse{}, err
	}
	if o.overWS {
		var out types.BalanceUpdatesResponse
		return out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetBalanceUpdates(ctx, req)
}

func (c *Client) GetTransfers(ctx context.Context, opts ...Option) (types.TransfersResponse, error) {
	req, o, subID, err := c.signedRead(ctx, "getTransfers", opts...)
	if err != nil {
		return types.TransfersResponse{}, err
	}
	if o.overWS {
		var out types.TransfersResponse
		return out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetTransfers(ctx, req)
}

func (c *Client) GetPositionHistory(ctx context.Context, opts ...Option) (types.PositionHistoryResponse, error) {
	req, o, subID, err := c.signedRead(ctx, "getPositionHistory", opts...)
	if err != nil {
		return types.PositionHistoryResponse{}, err
	}
	if o.overWS {
		var out types.PositionHistoryResponse
		return out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetPositionHistory(ctx, req)
}

func (c *Client) GetPerformanceHistory(ctx context.Context, opts ...Option) (*types.PerformanceHistoryResponse, error) {
	req, o, subID, err := c.signedRead(ctx, "getPerformanceHistory", opts...)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.PerformanceHistoryResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetPerformanceHistory(ctx, req)
}

func (c *Client) GetFees(ctx context.Context, opts ...Option) (*types.FeesResponse, error) {
	req, o, subID, err := c.signedRead(ctx, "getFees", opts...)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.FeesResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetFees(ctx, req)
}

func (c *Client) GetFundingPayments(ctx context.Context, opts ...Option) (*types.FundingPaymentsResponse, error) {
	req, o, subID, err := c.signedRead(ctx, "getFundingPayments", opts...)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.FundingPaymentsResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetFundingPayments(ctx, req)
}

func (c *Client) GetRateLimits(ctx context.Context, opts ...Option) (*types.RateLimitsResponse, error) {
	req, o, subID, err := c.signedRead(ctx, "getRateLimits", opts...)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.RateLimitsResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetRateLimits(ctx, req)
}

// CreateSubaccount creates a new subaccount using the selected master
// subaccount as proof of ownership.
func (c *Client) CreateSubaccount(ctx context.Context, name string, opts ...Option) (*types.CreateSubaccountResponse, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, err
	}
	req, err := s.SignCreateSubaccount(subID, name, 0, o.expiresAfterMs)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.CreateSubaccountResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.CreateSubaccount(ctx, req)
}

func (c *Client) UpdateSubAccountName(ctx context.Context, name string, opts ...Option) (*types.UpdateSubAccountNameResponse, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, err
	}
	req, err := s.SignUpdateSubAccountName(subID, name, 0, o.expiresAfterMs)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.UpdateSubAccountNameResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.UpdateSubAccountName(ctx, req)
}

func (c *Client) UpdateLeverage(ctx context.Context, symbol, leverage string, opts ...Option) (*types.UpdateLeverageResponse, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, err
	}
	req, err := s.SignUpdateLeverage(subID, symbol, leverage, 0, o.expiresAfterMs)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.UpdateLeverageResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.UpdateLeverage(ctx, req)
}

func (c *Client) WithdrawCollateral(ctx context.Context, symbol, amount, destination string, opts ...Option) (*types.WithdrawCollateralResponse, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, err
	}
	req, err := s.SignWithdrawCollateral(subID, symbol, amount, destination, 0, o.expiresAfterMs)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.WithdrawCollateralResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.WithdrawCollateral(ctx, req)
}

func (c *Client) TransferCollateral(ctx context.Context, toSubAccountID uint64, symbol, amount string, opts ...Option) (*types.TransferCollateralResponse, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, err
	}
	req, err := s.SignTransferCollateral(subID, toSubAccountID, symbol, amount, 0, o.expiresAfterMs)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.TransferCollateralResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.TransferCollateral(ctx, req)
}

func (c *Client) AddDelegatedSigner(ctx context.Context, delegateAddress string, permissions []string, expiresAt int64, opts ...Option) (*types.AddDelegatedSignerResponse, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, err
	}
	req, err := s.SignAddDelegatedSigner(subID, delegateAddress, permissions, expiresAt, 0, o.expiresAfterMs)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.AddDelegatedSignerResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.AddDelegatedSigner(ctx, req)
}

func (c *Client) GetDelegatedSigners(ctx context.Context, opts ...Option) (types.DelegatedSignersResponse, error) {
	req, o, subID, err := c.signedRead(ctx, "getDelegatedSigners", opts...)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.DelegatedSignersResponse
		return out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetDelegatedSigners(ctx, req)
}

func (c *Client) RemoveDelegatedSigner(ctx context.Context, delegateAddress string, opts ...Option) (*types.RemoveDelegatedSignerResponse, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, err
	}
	req, err := s.SignRemoveDelegatedSigner(subID, delegateAddress, 0, o.expiresAfterMs)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.RemoveDelegatedSignerResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.RemoveDelegatedSigner(ctx, req)
}

func (c *Client) RemoveAllDelegatedSigners(ctx context.Context, opts ...Option) (*types.RemoveAllDelegatedSignersResponse, error) {
	o := c.options(opts...)
	s, err := c.requireSigner()
	if err != nil {
		return nil, err
	}
	subID, err := c.resolveSubAccount(ctx, o)
	if err != nil {
		return nil, err
	}
	req, err := s.SignRemoveAllDelegatedSigners(subID, 0, o.expiresAfterMs)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.RemoveAllDelegatedSignersResponse
		return &out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.RemoveAllDelegatedSigners(ctx, req)
}

func (c *Client) GetDelegationsForDelegate(ctx context.Context, opts ...Option) (types.DelegationsForDelegateResponse, error) {
	req, o, subID, err := c.signedRead(ctx, "getDelegationsForDelegate", opts...)
	if err != nil {
		return nil, err
	}
	if o.overWS {
		var out types.DelegationsForDelegateResponse
		return out, c.postWS(ctx, subID, req, &out)
	}
	return c.trade.GetDelegationsForDelegate(ctx, req)
}

func stringsEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
