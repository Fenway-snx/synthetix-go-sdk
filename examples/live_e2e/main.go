package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Fenway-snx/synthetix-go-sdk/synthetix"
	"github.com/Fenway-snx/synthetix-go-sdk/types"
)

const (
	defaultTimeout     = 5 * time.Minute
	defaultStepTimeout = 30 * time.Second
)

func main() {
	if os.Getenv("SYNTHETIX_LIVE_E2E") != "1" {
		log.Fatal("set SYNTHETIX_LIVE_E2E=1 to run live e2e; this uses real credentials and places real orders")
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	c, err := synthetix.NewClientFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	symbol := env("SYNTHETIX_E2E_SYMBOL", "ETH-USDT")
	leverage := env("SYNTHETIX_E2E_LEVERAGE", "5")
	orderbookDepth := envInt("SYNTHETIX_E2E_ORDERBOOK_DEPTH", 5)
	prefix := fmt.Sprintf("go-live-e2e-%d", time.Now().UnixMilli())
	cleanupCloids := make([]string, 0, 8)
	marketOpened := false
	var marketQty string

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		for i := len(cleanupCloids) - 1; i >= 0; i-- {
			cloid := cleanupCloids[i]
			_, err := c.CancelOrderByCloid(cleanupCtx, cloid)
			if err != nil {
				fmt.Printf("[cleanup] cancel %s: %v\n", cloid, err)
			}
		}
		if marketOpened {
			_, err := c.MarketOrder(cleanupCtx, symbol, synthetix.SideSell, marketQty, synthetix.WithReduceOnly(), synthetix.WithClientOrderID(prefix+"-market-close-cleanup"))
			if err != nil {
				fmt.Printf("[cleanup] close market position: %v\n", err)
			}
		}
	}()

	fmt.Printf("live e2e: base=%s symbol=%s prefix=%s\n", env("SYNTHETIX_BASE_URL", synthetix.DefaultBaseURL), symbol, prefix)
	fmt.Println("WARNING: this places and cancels real orders, and opens/closes one tiny market position.")

	market, bid, ask, err := loadMarketContext(ctx, c, symbol, orderbookDepth)
	if err != nil {
		log.Fatal(err)
	}
	quantity := env("SYNTHETIX_E2E_QUANTITY", market.MinOrderSize)
	if quantity == "" {
		log.Fatal("set SYNTHETIX_E2E_QUANTITY; market min order size was empty")
	}
	limitBuyPrice, err := formatMarketPrice(ctx, c, symbol, bid.Mul(decimal.RequireFromString("0.5")))
	if err != nil {
		log.Fatal(err)
	}
	modifiedLimitBuyPrice, err := formatMarketPrice(ctx, c, symbol, bid.Mul(decimal.RequireFromString("0.45")))
	if err != nil {
		log.Fatal(err)
	}
	triggerSellPrice, err := formatMarketPrice(ctx, c, symbol, ask.Mul(decimal.RequireFromString("2")))
	if err != nil {
		log.Fatal(err)
	}
	triggerStopPrice, err := formatMarketPrice(ctx, c, symbol, bid.Mul(decimal.RequireFromString("0.5")))
	if err != nil {
		log.Fatal(err)
	}
	marketQty = quantity

	steps := []step{
		{"GetExchangeStatus", false, func(ctx context.Context) error {
			status, err := c.GetExchangeStatus(ctx)
			if err != nil {
				return err
			}
			if !status.IsRunning() {
				return fmt.Errorf("exchange not running: %+v", status)
			}
			return nil
		}},
		{"GetMarkets", false, func(ctx context.Context) error {
			markets, err := c.Info().GetMarkets(ctx, true)
			if err != nil {
				return err
			}
			if len(markets) == 0 {
				return errors.New("no active markets returned")
			}
			return nil
		}},
		{"GetMarketPrices", false, func(ctx context.Context) error {
			prices, err := c.Info().GetMarketPrices(ctx)
			if err != nil {
				return err
			}
			if _, ok := prices[symbol]; !ok {
				return fmt.Errorf("no price for %s", symbol)
			}
			return nil
		}},
		{"GetOrderbook", false, func(ctx context.Context) error {
			book, err := c.Info().GetOrderbook(ctx, symbol, orderbookDepth)
			if err != nil {
				return err
			}
			if len(book.Bids) == 0 || len(book.Asks) == 0 {
				return errors.New("orderbook missing bids or asks")
			}
			return nil
		}},
		{"ValidateAuth", false, func(ctx context.Context) error {
			status, err := c.ValidateAuth(ctx)
			if err != nil {
				return err
			}
			if !status.Ready {
				return fmt.Errorf("auth not ready: %+v", status)
			}
			return nil
		}},
		{"GetSubAccount", false, func(ctx context.Context) error {
			_, err := c.GetSubAccount(ctx)
			return err
		}},
		{"GetOpenOrders", false, func(ctx context.Context) error {
			_, err := c.GetOpenOrders(ctx)
			return err
		}},
		{"GetPositions", false, func(ctx context.Context) error {
			_, err := c.GetPositions(ctx)
			return err
		}},
		{"GetPortfolio", false, func(ctx context.Context) error {
			_, err := c.GetPortfolio(ctx)
			return err
		}},
		{"GetRateLimits", false, func(ctx context.Context) error {
			_, err := c.GetRateLimits(ctx)
			return err
		}},
		{"UpdateLeverage", false, func(ctx context.Context) error {
			_, err := c.UpdateLeverage(ctx, symbol, leverage)
			return err
		}},
		{"REST limit place/modify/cancel", false, func(ctx context.Context) error {
			cloid := prefix + "-rest-limit"
			cleanupCloids = append(cleanupCloids, cloid)
			resp, err := c.LimitGTCOrder(ctx, symbol, synthetix.SideBuy, limitBuyPrice, quantity, synthetix.WithPostOnly(), synthetix.WithClientOrderID(cloid))
			if err != nil {
				return err
			}
			if err := requireResting(resp, cloid); err != nil {
				return err
			}
			printOrderStatus("rest-limit-place", cloid, resp.Statuses[0])
			modified, err := c.ModifyOrderByCloid(ctx, cloid, modifiedLimitBuyPrice, quantity, "")
			if err != nil {
				return err
			}
			if err := requireModifyOK(modified, cloid); err != nil {
				return err
			}
			fmt.Printf("       modified %s price=%s\n", cloid, modifiedLimitBuyPrice)
			if _, err = c.CancelOrderByCloid(ctx, cloid); err != nil {
				return err
			}
			cleanupCloids = removeCloid(cleanupCloids, cloid)
			return nil
		}},
		{"WS limit place/modify/cancel", false, func(ctx context.Context) error {
			cloid := prefix + "-ws-limit"
			cleanupCloids = append(cleanupCloids, cloid)
			resp, err := c.LimitGTCOrder(ctx, symbol, synthetix.SideBuy, limitBuyPrice, quantity, synthetix.WithPostOnly(), synthetix.WithClientOrderID(cloid), synthetix.OverWS())
			if err != nil {
				return err
			}
			if err := requireResting(resp, cloid); err != nil {
				return err
			}
			printOrderStatus("ws-limit-place", cloid, resp.Statuses[0])
			modified, err := c.ModifyOrderByCloid(ctx, cloid, modifiedLimitBuyPrice, quantity, "", synthetix.OverWS())
			if err != nil {
				return err
			}
			if err := requireModifyOK(modified, cloid); err != nil {
				return err
			}
			fmt.Printf("       modified %s price=%s over ws\n", cloid, modifiedLimitBuyPrice)
			if _, err = c.CancelOrderByCloid(ctx, cloid, synthetix.OverWS()); err != nil {
				return err
			}
			cleanupCloids = removeCloid(cleanupCloids, cloid)
			return nil
		}},
		{"TWAP place/cancel", true, func(ctx context.Context) error {
			cloid := prefix + "-twap"
			resp, err := c.TwapOrder(ctx, symbol, synthetix.SideBuy, quantity, 300, limitBuyPrice, synthetix.WithIntervalSeconds(30), synthetix.WithClientOrderID(cloid))
			if err != nil {
				return err
			}
			if err := requireAccepted(resp, cloid); err != nil {
				return err
			}
			cleanupCloids = append(cleanupCloids, cloid)
			printOrderStatus("twap-place", cloid, resp.Statuses[0])
			if _, err = c.CancelOrderByCloid(ctx, cloid); err != nil {
				return err
			}
			cleanupCloids = removeCloid(cleanupCloids, cloid)
			return nil
		}},
		{"Market open, conditionals, close", false, func(ctx context.Context) error {
			openCloid := prefix + "-market-open"
			closeCloid := prefix + "-market-close"
			openResp, err := c.MarketOrder(ctx, symbol, synthetix.SideBuy, quantity, synthetix.WithClientOrderID(openCloid))
			if err != nil {
				return err
			}
			if err := requireFilled(openResp, openCloid); err != nil {
				return err
			}
			printOrderStatus("market-open", openCloid, openResp.Statuses[0])
			marketOpened = true
			tpCloid := prefix + "-trigger-tp"
			slCloid := prefix + "-trigger-sl"
			cleanupCloids = append(cleanupCloids, tpCloid, slCloid)
			tpResp, err := c.TriggerTPOrder(ctx, symbol, synthetix.SideSell, "", triggerSellPrice, "0", synthetix.WithTriggerMarket(), synthetix.WithClosePosition(), synthetix.WithClientOrderID(tpCloid))
			if err != nil {
				return err
			}
			if err := requireAccepted(tpResp, tpCloid); err != nil {
				return err
			}
			printOrderStatus("trigger-tp-place", tpCloid, tpResp.Statuses[0])
			slResp, err := c.TriggerSLOrder(ctx, symbol, synthetix.SideSell, "", triggerStopPrice, "0", synthetix.WithTriggerMarket(), synthetix.WithClosePosition(), synthetix.WithClientOrderID(slCloid))
			if err != nil {
				return err
			}
			if err := requireAccepted(slResp, slCloid); err != nil {
				return err
			}
			printOrderStatus("trigger-sl-place", slCloid, slResp.Statuses[0])
			if _, err = c.CancelOrdersByCloid(ctx, []string{tpCloid, slCloid}); err != nil {
				return err
			}
			cleanupCloids = removeCloid(removeCloid(cleanupCloids, tpCloid), slCloid)
			closeResp, err := c.MarketOrder(ctx, symbol, synthetix.SideSell, quantity, synthetix.WithReduceOnly(), synthetix.WithClientOrderID(closeCloid))
			if err != nil {
				return err
			}
			if err := requireFilled(closeResp, closeCloid); err != nil {
				return err
			}
			printOrderStatus("market-close", closeCloid, closeResp.Statuses[0])
			marketOpened = false
			return nil
		}},
		{"Post-trade reads", false, func(ctx context.Context) error {
			if _, err := c.GetOrderHistory(ctx, synthetix.WithOrderHistoryFilter(time.Now().Add(-10*time.Minute).UnixMilli(), time.Now().Add(time.Minute).UnixMilli(), 50)); err != nil {
				return err
			}
			if _, err := c.GetTrades(ctx); err != nil {
				return err
			}
			if _, err := c.GetBalanceUpdates(ctx, synthetix.WithBalanceUpdatesFilter(time.Now().Add(-10*time.Minute).UnixMilli(), time.Now().Add(time.Minute).UnixMilli(), "", 50, 0)); err != nil {
				return err
			}
			if _, err := c.GetFees(ctx); err != nil {
				return err
			}
			if _, err := c.GetFundingPayments(ctx); err != nil {
				return err
			}
			return nil
		}},
	}

	started := time.Now()
	for _, s := range steps {
		if err := runStep(ctx, s); err != nil {
			log.Fatalf("%s failed: %v", s.name, err)
		}
	}
	fmt.Printf("[OK] live e2e completed in %s\n", time.Since(started).Round(time.Millisecond))
}

type step struct {
	name     string
	optional bool
	fn       func(context.Context) error
}

func runStep(ctx context.Context, s step) error {
	started := time.Now()
	stepCtx, cancel := context.WithTimeout(ctx, defaultStepTimeout)
	defer cancel()
	err := s.fn(stepCtx)
	elapsed := time.Since(started).Round(time.Millisecond)
	if err != nil {
		if s.optional {
			fmt.Printf("[SKIP] %-34s %s (%v)\n", s.name, elapsed, err)
			return nil
		}
		fmt.Printf("[FAIL] %-34s %s\n", s.name, elapsed)
		return err
	}
	fmt.Printf("[OK]   %-34s %s\n", s.name, elapsed)
	return nil
}

func loadMarketContext(ctx context.Context, c *synthetix.Client, symbol string, orderbookDepth int) (*types.MarketResponse, decimal.Decimal, decimal.Decimal, error) {
	market, err := c.Info().GetMarket(ctx, symbol)
	if err != nil {
		return nil, decimal.Zero, decimal.Zero, err
	}
	book, err := c.Info().GetOrderbook(ctx, symbol, orderbookDepth)
	if err != nil {
		return nil, decimal.Zero, decimal.Zero, err
	}
	if len(book.Bids) == 0 || len(book.Asks) == 0 {
		return nil, decimal.Zero, decimal.Zero, errors.New("orderbook missing bids or asks")
	}
	bid, err := decimal.NewFromString(book.Bids[0].Price)
	if err != nil {
		return nil, decimal.Zero, decimal.Zero, err
	}
	ask, err := decimal.NewFromString(book.Asks[0].Price)
	if err != nil {
		return nil, decimal.Zero, decimal.Zero, err
	}
	return market, bid, ask, nil
}

func formatMarketPrice(ctx context.Context, c *synthetix.Client, symbol string, price decimal.Decimal) (string, error) {
	return c.FormatPrice(ctx, symbol, price.String())
}

func requireAccepted(resp *types.PlaceOrdersResponse, cloid string) error {
	status, err := singleStatus(resp)
	if err != nil {
		return err
	}
	if status.Error != "" {
		return fmt.Errorf("%s rejected: %s (%s)", cloid, status.Error, status.ErrorCode)
	}
	return nil
}

func requireResting(resp *types.PlaceOrdersResponse, cloid string) error {
	status, err := singleStatus(resp)
	if err != nil {
		return err
	}
	if status.Error != "" {
		return fmt.Errorf("%s rejected: %s (%s)", cloid, status.Error, status.ErrorCode)
	}
	if status.Resting == nil {
		return fmt.Errorf("%s did not rest: %+v", cloid, status)
	}
	return nil
}

func requireFilled(resp *types.PlaceOrdersResponse, cloid string) error {
	status, err := singleStatus(resp)
	if err != nil {
		return err
	}
	if status.Error != "" {
		return fmt.Errorf("%s rejected: %s (%s)", cloid, status.Error, status.ErrorCode)
	}
	if status.Filled == nil {
		return fmt.Errorf("%s did not fill: %+v", cloid, status)
	}
	return nil
}

func requireModifyOK(resp *types.ModifyOrderResponse, cloid string) error {
	if resp == nil {
		return errors.New("empty modify response")
	}
	if resp.Error != "" {
		return fmt.Errorf("%s modify rejected: %s (%s)", cloid, resp.Error, resp.ErrorCode)
	}
	return nil
}

func printOrderStatus(label, cloid string, status types.OrderStatus) {
	venueID := ""
	state := "accepted"
	switch {
	case status.Resting != nil:
		state = "resting"
		venueID = status.Resting.OrderID.VenueID
	case status.Filled != nil:
		state = "filled"
		venueID = status.Filled.OrderID.VenueID
	case status.Canceled != nil:
		state = "canceled"
		venueID = status.Canceled.OrderID.VenueID
	case status.Error != "":
		state = "error:" + status.Error
	}
	fmt.Printf("       %-18s cloid=%s venueId=%s state=%s\n", label, cloid, venueID, state)
}

func singleStatus(resp *types.PlaceOrdersResponse) (types.OrderStatus, error) {
	if resp == nil || len(resp.Statuses) == 0 {
		return types.OrderStatus{}, errors.New("empty place order response")
	}
	return resp.Statuses[0], nil
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	var out int
	if _, err := fmt.Sscanf(value, "%d", &out); err != nil || out <= 0 {
		return fallback
	}
	return out
}

func removeCloid(cloids []string, cloid string) []string {
	for i := range cloids {
		if cloids[i] == cloid {
			return append(cloids[:i], cloids[i+1:]...)
		}
	}
	return cloids
}
