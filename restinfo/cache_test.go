package restinfo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// marketsServer returns a server that always responds to
// getMarkets with a two-market snapshot. It tracks hits atomically
// so cache-hit/cache-miss tests can assert on upstream call counts.
func marketsServer(t *testing.T, ttl time.Duration) (*Client, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		body, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if decoded["action"] != "getMarkets" {
			t.Errorf("unexpected action %v", decoded["action"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"requestId":"r","response":[{"symbol":"BTC-USDT","description":"Bitcoin","baseAsset":"BTC","quoteAsset":"USDT","isOpen":true},{"symbol":"ETH-USDT","description":"Ether","baseAsset":"ETH","quoteAsset":"USDT","isOpen":true}]}`)
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient(Config{
		BaseURL:        srv.URL,
		HTTPClient:     srv.Client(),
		MarketCacheTTL: ttl,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client, &hits
}

func TestGetMarket_CachesAcrossCalls(t *testing.T) {
	client, hits := marketsServer(t, time.Minute)

	for i := 0; i < 5; i++ {
		m, err := client.GetMarket(context.Background(), "BTC-USDT")
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		if m.Symbol != "BTC-USDT" {
			t.Fatalf("wrong market: %+v", m)
		}
	}
	if n := atomic.LoadInt32(hits); n != 1 {
		t.Errorf("expected 1 upstream call across 5 GetMarket, got %d", n)
	}
}

func TestGetMarket_ExpiresAfterTTL(t *testing.T) {
	client, hits := marketsServer(t, 20*time.Millisecond)

	if _, err := client.GetMarket(context.Background(), "BTC-USDT"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := client.GetMarket(context.Background(), "BTC-USDT"); err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(hits); n != 2 {
		t.Errorf("expected 2 upstream calls after TTL expiry, got %d", n)
	}
}

func TestGetMarket_InvalidateForcesRefetch(t *testing.T) {
	client, hits := marketsServer(t, time.Minute)

	if _, err := client.GetMarket(context.Background(), "BTC-USDT"); err != nil {
		t.Fatal(err)
	}
	client.InvalidateMarketCache()
	if _, err := client.GetMarket(context.Background(), "BTC-USDT"); err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(hits); n != 2 {
		t.Errorf("expected 2 upstream calls after invalidate, got %d", n)
	}
}

func TestGetMarket_NegativeTTLDisablesCache(t *testing.T) {
	client, hits := marketsServer(t, -1)

	for i := 0; i < 3; i++ {
		if _, err := client.GetMarket(context.Background(), "BTC-USDT"); err != nil {
			t.Fatal(err)
		}
	}
	if n := atomic.LoadInt32(hits); n != 3 {
		t.Errorf("expected 3 upstream calls (no cache), got %d", n)
	}
}

func TestGetMarket_NotFoundReturnsTypedErr(t *testing.T) {
	client, _ := marketsServer(t, time.Minute)

	_, err := client.GetMarket(context.Background(), "DOGE-USDT")
	if err == nil {
		t.Fatal("expected error")
	}
	var nfErr ErrMarketNotFound
	if !errors.As(err, &nfErr) {
		t.Fatalf("want ErrMarketNotFound, got %T: %v", err, err)
	}
	if nfErr.Symbol != "DOGE-USDT" {
		t.Errorf("symbol: %q", nfErr.Symbol)
	}
}

// TestGetMarket_ConcurrentColdCache_DoesNotThunder verifies that N
// concurrent callers with a cold cache do not each issue their own
// upstream request. Exact fanout is timing-dependent but must be
// bounded well below N; empirically it should be 1 under the RWMutex
// protection we have.
func TestGetMarket_ConcurrentColdCache_DoesNotThunder(t *testing.T) {
	client, hits := marketsServer(t, time.Minute)

	const N = 20
	var wg sync.WaitGroup
	wg.Add(N)
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			<-start
			_, err := client.GetMarket(context.Background(), "BTC-USDT")
			if err != nil {
				t.Errorf("concurrent GetMarket: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	// We accept up to 2 upstream calls because two goroutines could
	// both pass the RLock fresh check before either grabs the write
	// lock. More than 2 means our single-flight property is broken.
	if n := atomic.LoadInt32(hits); n > 2 {
		t.Errorf("expected at most 2 upstream calls under concurrency, got %d", n)
	}
}

// TestGetMarket_RefetchOnError does NOT cache failure responses
// (we only cache on success). Verify that a failed first attempt
// is retried cleanly on the next call.
func TestGetMarket_RefetchOnError(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			w.WriteHeader(500)
			_, _ = io.WriteString(w, `{"requestId":"r","error":{"code":"INTERNAL","message":"boom"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"requestId":"r","response":[{"symbol":"BTC-USDT","description":"B","baseAsset":"BTC","quoteAsset":"USDT","isOpen":true}]}`)
	}))
	t.Cleanup(srv.Close)
	client, _ := NewClient(Config{BaseURL: srv.URL, HTTPClient: srv.Client(), MarketCacheTTL: time.Minute})

	if _, err := client.GetMarket(context.Background(), "BTC-USDT"); err == nil {
		t.Fatal("expected first call to fail")
	}
	m, err := client.GetMarket(context.Background(), "BTC-USDT")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if m.Symbol != "BTC-USDT" {
		t.Errorf("symbol %q", m.Symbol)
	}
	if n := atomic.LoadInt32(&hits); n != 2 {
		t.Errorf("expected 2 upstream calls (error not cached), got %d", n)
	}
	_ = fmt.Sprintf
}
