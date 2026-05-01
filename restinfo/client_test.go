package restinfo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// testServer spins up an httptest.Server that expects POSTs to
// /v1/info and dispatches by action. responder returns (statusCode,
// responseBody) for a given decoded request body.
func testServer(t *testing.T, responder func(t *testing.T, req map[string]any) (int, string)) (*httptest.Server, *Client, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/info" {
			t.Errorf("expected /v1/info, got %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		code, payload := responder(t, decoded)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = io.WriteString(w, payload)
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient(Config{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return srv, client, &hits
}

func TestNewClient_RequiresBaseURL(t *testing.T) {
	if _, err := NewClient(Config{}); err == nil {
		t.Fatal("expected error for empty BaseURL")
	}
}

func TestNewClient_DefaultsApplied(t *testing.T) {
	c, err := NewClient(Config{BaseURL: "https://example.invalid"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.userAgent != DefaultUserAgent {
		t.Errorf("default UserAgent: got %q want %q", c.userAgent, DefaultUserAgent)
	}
	if c.marketCacheTTL != DefaultMarketCacheTTL {
		t.Errorf("default MarketCacheTTL: got %v want %v", c.marketCacheTTL, DefaultMarketCacheTTL)
	}
	if c.httpClient == nil || c.httpClient.Timeout != DefaultHTTPTimeout {
		t.Errorf("default HTTPTimeout: want %v", DefaultHTTPTimeout)
	}
	if !strings.HasPrefix(c.baseURL, "https://") || strings.HasSuffix(c.baseURL, "/") {
		t.Errorf("baseURL normalization failed: %q", c.baseURL)
	}
}

func TestGetExchangeStatus_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/exchange/status" {
			t.Errorf("expected /v1/exchange/status, got %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"accepting_orders":true,"exchange_status":"RUNNING","code":"","message":"OK","timestamp_ms":1700000000000}`)
	}))
	t.Cleanup(srv.Close)
	client, _ := NewClient(Config{BaseURL: srv.URL, HTTPClient: srv.Client()})

	got, err := client.GetExchangeStatus(context.Background())
	if err != nil {
		t.Fatalf("GetExchangeStatus: %v", err)
	}
	if !got.IsRunning() {
		t.Error("expected IsRunning true")
	}
	if got.IsDegraded() {
		t.Error("expected IsDegraded false")
	}
	if got.Message != "OK" {
		t.Errorf("message: got %q", got.Message)
	}
}

func TestGetWSExchangeStatus_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ws/exchange/status" {
			t.Errorf("expected /v1/ws/exchange/status, got %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"requestId":"req-1","response":{"accepting_orders":true,"exchange_status":"RUNNING","code":"","message":"OK","timestamp_ms":1700000000000}}`)
	}))
	t.Cleanup(srv.Close)
	client, _ := NewClient(Config{BaseURL: srv.URL, HTTPClient: srv.Client()})

	got, err := client.GetWSExchangeStatus(context.Background())
	if err != nil {
		t.Fatalf("GetWSExchangeStatus: %v", err)
	}
	if !got.IsRunning() {
		t.Error("expected IsRunning true")
	}
}

func TestGetMarketPrices_ParsesMap(t *testing.T) {
	_, client, _ := testServer(t, func(t *testing.T, req map[string]any) (int, string) {
		if req["action"] != "getMarketPrices" {
			t.Errorf("unexpected action %v", req["action"])
		}
		return 200, `{"requestId":"r","response":{"BTC-USDT":{"symbol":"BTC-USDT","bestBid":"1","bestAsk":"2","markPrice":"1.5","indexPrice":"1.4","lastPrice":"1.5","prevDayPrice":"1","volume24h":"100","quoteVolume24h":"150","fundingRate":"0.0001","openInterest":"500","timestamp":1700000000}}}`
	})
	got, err := client.GetMarketPrices(context.Background())
	if err != nil {
		t.Fatalf("GetMarketPrices: %v", err)
	}
	btc, ok := got["BTC-USDT"]
	if !ok {
		t.Fatal("missing BTC-USDT")
	}
	if btc.FundingRate != "0.0001" {
		t.Errorf("FundingRate: got %q", btc.FundingRate)
	}
}

func TestGetOrderbook_ParsesArrayLevels(t *testing.T) {
	_, client, _ := testServer(t, func(t *testing.T, req map[string]any) (int, string) {
		if req["symbol"] != "ETH-USDT" {
			t.Errorf("symbol: got %v", req["symbol"])
		}
		if req["limit"].(float64) != 10 {
			t.Errorf("limit: got %v", req["limit"])
		}
		return 200, `{"requestId":"r","response":{"symbol":"ETH-USDT","bids":[["100","1"],["99","2"]],"asks":[["101","1.5"]],"timestamp":1700000000000}}`
	})
	got, err := client.GetOrderbook(context.Background(), "ETH-USDT", 10)
	if err != nil {
		t.Fatalf("GetOrderbook: %v", err)
	}
	if len(got.Bids) != 2 || got.Bids[0].Price != "100" || got.Bids[0].Quantity != "1" {
		t.Errorf("bids: %+v", got.Bids)
	}
	if len(got.Asks) != 1 || got.Asks[0].Price != "101" {
		t.Errorf("asks: %+v", got.Asks)
	}
}

func TestGetCandles_OmitsZeroOptionals(t *testing.T) {
	_, client, _ := testServer(t, func(t *testing.T, req map[string]any) (int, string) {
		if _, has := req["limit"]; has {
			t.Error("limit should be omitted when 0")
		}
		if _, has := req["startTime"]; has {
			t.Error("startTime should be omitted when 0")
		}
		if req["interval"] != "1h" {
			t.Errorf("interval: got %v", req["interval"])
		}
		return 200, `{"requestId":"r","response":{"symbol":"BTC-USDT","interval":"1h","candles":[{"openTime":1,"closeTime":2,"openPrice":"100","highPrice":"101","lowPrice":"99","closePrice":"100.5","volume":"10","quoteVolume":"1000","tradeCount":5}]}}`
	})
	got, err := client.GetCandles(context.Background(), "BTC-USDT", "1h", 0, 0, 0)
	if err != nil {
		t.Fatalf("GetCandles: %v", err)
	}
	if len(got.Candles) != 1 || got.Candles[0].TradeCount != 5 {
		t.Errorf("candles: %+v", got.Candles)
	}
}

func TestGetLastTrades_OptionalOffset(t *testing.T) {
	_, client, _ := testServer(t, func(t *testing.T, req map[string]any) (int, string) {
		if req["offset"].(float64) != 20 {
			t.Errorf("offset: got %v", req["offset"])
		}
		return 200, `{"requestId":"r","response":{"symbol":"S","trades":[{"tradeId":"t1","symbol":"S","price":"1","quantity":"2","side":"BUY","timestampMs":1}]}}`
	})
	got, err := client.GetLastTrades(context.Background(), "S", 50, 20)
	if err != nil {
		t.Fatalf("GetLastTrades: %v", err)
	}
	if len(got.Trades) != 1 || got.Trades[0].Side != "BUY" {
		t.Errorf("trades: %+v", got.Trades)
	}
}

func TestGetFundingRate(t *testing.T) {
	_, client, _ := testServer(t, func(t *testing.T, req map[string]any) (int, string) {
		if req["symbol"] != "SOL-USDT" {
			t.Errorf("symbol %v", req["symbol"])
		}
		return 200, `{"requestId":"r","response":{"symbol":"SOL-USDT","estimatedFundingRate":"0.0002","nextFundingTimeMs":1700000000000}}`
	})
	got, err := client.GetFundingRate(context.Background(), "SOL-USDT")
	if err != nil {
		t.Fatalf("GetFundingRate: %v", err)
	}
	if got.EstimatedFundingRate != "0.0002" {
		t.Errorf("rate %q", got.EstimatedFundingRate)
	}
}

func TestGetFundingRateHistory(t *testing.T) {
	_, client, _ := testServer(t, func(t *testing.T, req map[string]any) (int, string) {
		if req["action"] != "getFundingRateHistory" {
			t.Errorf("action %v", req["action"])
		}
		if req["symbol"] != "SOL-USDT" || req["limit"].(float64) != 2 {
			t.Errorf("request %+v", req)
		}
		return 200, `{"requestId":"r","response":{"symbol":"SOL-USDT","history":[{"symbol":"SOL-USDT","fundingRate":"0.0002","timestamp":1700000000000}]}}`
	})
	got, err := client.GetFundingRateHistory(context.Background(), "SOL-USDT", 2, 1, 2)
	if err != nil {
		t.Fatalf("GetFundingRateHistory: %v", err)
	}
	if got.Symbol != "SOL-USDT" || len(got.History) != 1 || got.History[0].FundingRate != "0.0002" {
		t.Errorf("history %+v", got)
	}
}

func TestGetOpenInterest(t *testing.T) {
	_, client, _ := testServer(t, func(t *testing.T, _ map[string]any) (int, string) {
		return 200, `{"requestId":"r","response":[{"symbol":"BTC-USDT","openInterest":"123","longOpenInterest":"60","shortOpenInterest":"63","timestamp":1}]}`
	})
	got, err := client.GetOpenInterest(context.Background())
	if err != nil {
		t.Fatalf("GetOpenInterest: %v", err)
	}
	if len(got) != 1 || got[0].LongOpenInterest != "60" {
		t.Errorf("open interest: %+v", got)
	}
}

func TestGetMids(t *testing.T) {
	_, client, _ := testServer(t, func(t *testing.T, _ map[string]any) (int, string) {
		return 200, `{"requestId":"r","response":{"BTC-USDT":{"symbol":"BTC-USDT","midPrice":"100.5"}}}`
	})
	got, err := client.GetMids(context.Background())
	if err != nil {
		t.Fatalf("GetMids: %v", err)
	}
	if got["BTC-USDT"].MidPrice != "100.5" {
		t.Errorf("mid %+v", got)
	}
}

func TestGetSubAccountIds_FlatList(t *testing.T) {
	_, client, _ := testServer(t, func(t *testing.T, req map[string]any) (int, string) {
		if _, has := req["includeDelegations"]; has {
			t.Error("includeDelegations should be omitted by default")
		}
		return 200, `{"requestId":"r","response":["1","2","3"]}`
	})
	got, err := client.GetSubAccountIds(context.Background(), "0xabc")
	if err != nil {
		t.Fatalf("GetSubAccountIds: %v", err)
	}
	if len(got) != 3 || got[1] != "2" {
		t.Errorf("ids: %+v", got)
	}
}

func TestGetSubAccountIdsWithDelegations(t *testing.T) {
	_, client, _ := testServer(t, func(t *testing.T, req map[string]any) (int, string) {
		if req["includeDelegations"] != true {
			t.Error("expected includeDelegations true")
		}
		return 200, `{"requestId":"r","response":{"subAccountIds":["1"],"delegatedSubAccountIds":["9","10"]}}`
	})
	got, err := client.GetSubAccountIdsWithDelegations(context.Background(), "0xabc")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got.DelegatedSubAccountIDs) != 2 {
		t.Errorf("delegations %+v", got)
	}
}

func TestRESTError_FromStructuredErrorBranch(t *testing.T) {
	_, client, _ := testServer(t, func(t *testing.T, _ map[string]any) (int, string) {
		return 400, `{"requestId":"r-err","error":{"code":"VALIDATION_ERROR","message":"bad symbol"}}`
	})
	_, err := client.GetOrderbook(context.Background(), "X", 0)
	if err == nil {
		t.Fatal("expected error")
	}
	var restErr *RESTError
	if !errors.As(err, &restErr) {
		t.Fatalf("expected *RESTError, got %T: %v", err, err)
	}
	if restErr.Code != "VALIDATION_ERROR" || restErr.StatusCode != 400 || restErr.RequestID != "r-err" {
		t.Errorf("unexpected RESTError: %+v", restErr)
	}
}

func TestTransportError_NonJSONBody(t *testing.T) {
	_, client, _ := testServer(t, func(t *testing.T, _ map[string]any) (int, string) {
		return 502, `<html>bad gateway</html>`
	})
	_, err := client.GetMarkets(context.Background(), false)
	if err == nil {
		t.Fatal("expected error")
	}
	var tErr *TransportError
	if !errors.As(err, &tErr) {
		t.Fatalf("expected *TransportError, got %T: %v", err, err)
	}
	if tErr.StatusCode != 502 {
		t.Errorf("status: got %d", tErr.StatusCode)
	}
}

func TestTransportError_4xxWithoutErrorEnvelope(t *testing.T) {
	_, client, _ := testServer(t, func(t *testing.T, _ map[string]any) (int, string) {
		return 404, `{"requestId":"r"}`
	})
	_, err := client.GetMarkets(context.Background(), false)
	var tErr *TransportError
	if !errors.As(err, &tErr) {
		t.Fatalf("expected *TransportError, got %T: %v", err, err)
	}
	if tErr.StatusCode != 404 {
		t.Errorf("status %d", tErr.StatusCode)
	}
}

func TestContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(250 * time.Millisecond)
		_, _ = io.WriteString(w, `{"requestId":"r","response":{}}`)
	}))
	t.Cleanup(srv.Close)
	client, _ := NewClient(Config{BaseURL: srv.URL, HTTPClient: srv.Client()})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := client.GetExchangeStatus(ctx)
	if err == nil {
		t.Fatal("expected timeout")
	}
	var tErr *TransportError
	if !errors.As(err, &tErr) {
		t.Fatalf("expected *TransportError, got %T", err)
	}
}

func TestValidation_EmptyArgs(t *testing.T) {
	client, _ := NewClient(Config{BaseURL: "https://example.invalid"})
	if _, err := client.GetMarket(context.Background(), ""); err == nil {
		t.Error("GetMarket empty symbol")
	}
	if _, err := client.GetOrderbook(context.Background(), "", 0); err == nil {
		t.Error("GetOrderbook empty symbol")
	}
	if _, err := client.GetCandles(context.Background(), "", "1h", 0, 0, 0); err == nil {
		t.Error("GetCandles empty symbol")
	}
	if _, err := client.GetCandles(context.Background(), "S", "", 0, 0, 0); err == nil {
		t.Error("GetCandles empty interval")
	}
	if _, err := client.GetLastTrades(context.Background(), "", 0, 0); err == nil {
		t.Error("GetLastTrades empty symbol")
	}
	if _, err := client.GetFundingRate(context.Background(), ""); err == nil {
		t.Error("GetFundingRate empty symbol")
	}
	if _, err := client.GetSubAccountIds(context.Background(), ""); err == nil {
		t.Error("GetSubAccountIds empty wallet")
	}
	if _, err := client.GetSubAccountIdsWithDelegations(context.Background(), ""); err == nil {
		t.Error("GetSubAccountIdsWithDelegations empty wallet")
	}
}
