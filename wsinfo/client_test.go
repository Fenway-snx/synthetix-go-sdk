package wsinfo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/synthetixio/synthetix-go/types"
)

// testServer wraps an httptest.Server hosting a /v1/ws/info upgrade
// handler. It exposes hooks to observe client requests and push
// arbitrary notifications.
type testServer struct {
	srv         *httptest.Server
	upgrader    websocket.Upgrader

	mu          sync.Mutex
	conns       []*websocket.Conn
	connWriteMu map[*websocket.Conn]*sync.Mutex
	received    []types.WSRequest
	onRequest   func(conn *websocket.Conn, req types.WSRequest)
	ackAll      bool
	connHook    func(*websocket.Conn)
	rejectNext  int32
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	ts := &testServer{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		ackAll:      true,
		connWriteMu: map[*websocket.Conn]*sync.Mutex{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ws/info", ts.handle)
	ts.srv = httptest.NewServer(mux)
	t.Cleanup(func() {
		ts.closeAll()
		ts.srv.Close()
	})
	return ts
}

func (ts *testServer) writeOn(conn *websocket.Conn, payload any) error {
	ts.mu.Lock()
	wm, ok := ts.connWriteMu[conn]
	if !ok {
		wm = &sync.Mutex{}
		ts.connWriteMu[conn] = wm
	}
	ts.mu.Unlock()
	wm.Lock()
	defer wm.Unlock()
	return conn.WriteJSON(payload)
}

func (ts *testServer) handle(w http.ResponseWriter, r *http.Request) {
	if atomic.LoadInt32(&ts.rejectNext) > 0 {
		atomic.AddInt32(&ts.rejectNext, -1)
		http.Error(w, "nope", http.StatusServiceUnavailable)
		return
	}
	conn, err := ts.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	ts.mu.Lock()
	ts.conns = append(ts.conns, conn)
	hook := ts.connHook
	ts.mu.Unlock()
	if hook != nil {
		hook(conn)
	}

	for {
		var req types.WSRequest
		if err := conn.ReadJSON(&req); err != nil {
			_ = conn.Close()
			return
		}
		ts.mu.Lock()
		ts.received = append(ts.received, req)
		handler := ts.onRequest
		ack := ts.ackAll
		ts.mu.Unlock()
		if handler != nil {
			handler(conn, req)
			continue
		}
		if ack {
			_ = ts.writeOn(conn, map[string]any{"id": req.ID, "status": 200, "result": map[string]any{"ok": true}})
		}
	}
}

func (ts *testServer) baseURL() string { return ts.srv.URL }

func (ts *testServer) latestConn() *websocket.Conn {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if len(ts.conns) == 0 {
		return nil
	}
	return ts.conns[len(ts.conns)-1]
}

func (ts *testServer) connCount() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.conns)
}

func (ts *testServer) requestCount() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.received)
}

func (ts *testServer) requestsByMethod(method string) []types.WSRequest {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	var out []types.WSRequest
	for _, r := range ts.received {
		if r.Method == method {
			out = append(out, r)
		}
	}
	return out
}

func (ts *testServer) pushNotification(conn *websocket.Conn, payload map[string]any) error {
	return ts.writeOn(conn, payload)
}

func (ts *testServer) closeAll() {
	ts.mu.Lock()
	conns := ts.conns
	ts.conns = nil
	ts.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

func (ts *testServer) dropActiveConnection() {
	ts.mu.Lock()
	conns := ts.conns
	ts.conns = nil
	ts.mu.Unlock()
	for _, c := range conns {
		_ = c.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseAbnormalClosure, "drop"),
			time.Now().Add(time.Second))
		_ = c.Close()
	}
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", d)
}

func fastClientConfig(baseURL string) Config {
	return Config{
		BaseURL:          baseURL,
		DialTimeout:      2 * time.Second,
		WriteTimeout:     500 * time.Millisecond,
		ReadTimeout:      2 * time.Second,
		ReconnectInitial: 10 * time.Millisecond,
		ReconnectMax:     40 * time.Millisecond,
		SubscribeTimeout: 500 * time.Millisecond,
		PingInterval:     -1,
	}
}

func TestToWSURL(t *testing.T) {
	cases := map[string]string{
		"http://api.synthetix.io":        "ws://api.synthetix.io/v1/ws/info",
		"https://api.synthetix.io/":      "wss://api.synthetix.io/v1/ws/info",
		"ws://localhost:8090":            "ws://localhost:8090/v1/ws/info",
		"wss://api.synthetix.io/prefix/": "wss://api.synthetix.io/prefix/v1/ws/info",
	}
	for in, want := range cases {
		got, err := toWSURL(in)
		if err != nil {
			t.Fatalf("toWSURL(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("toWSURL(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := toWSURL("ftp://bogus"); err == nil {
		t.Error("expected error for unsupported scheme")
	}
}

func TestSubscribeSpec_KeyIsStable(t *testing.T) {
	a := SubscribeSpec{Type: "orderbook", Symbol: "btc-usdt", Depth: 50, Format: "diff", UpdateFrequencyMs: 250}
	b := SubscribeSpec{Type: "orderbook", Symbol: "BTC-USDT ", Depth: 50, UpdateFrequencyMs: 250, Format: "diff"}
	if a.key() != b.key() {
		t.Errorf("keys differ:\n a=%s\n b=%s", a.key(), b.key())
	}
	c := SubscribeSpec{Type: "trade", Symbol: "BTC-USDT"}
	if a.key() == c.key() {
		t.Error("different channels should produce different keys")
	}
}

func TestSubscribe_HappyPath(t *testing.T) {
	ts := newTestServer(t)
	client, err := NewClient(fastClientConfig(ts.baseURL()))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	var got []*types.WSMessage
	var mu sync.Mutex
	unsub, err := client.Subscribe(context.Background(),
		SubscribeSpec{Type: types.WSSubscribeTrade, Symbol: "BTC-USDT"},
		func(m *types.WSMessage) {
			mu.Lock()
			defer mu.Unlock()
			got = append(got, m)
		})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	waitFor(t, 2*time.Second, func() bool { return ts.requestCount() >= 1 })

	subs := ts.requestsByMethod("subscribe")
	if len(subs) != 1 {
		t.Fatalf("want 1 subscribe, got %d", len(subs))
	}
	if subs[0].Params["type"] != "trade" || subs[0].Params["symbol"] != "BTC-USDT" {
		t.Errorf("unexpected params: %+v", subs[0].Params)
	}

	conn := ts.latestConn()
	_ = ts.pushNotification(conn, map[string]any{
		"method":  "trade",
		"channel": "trade",
		"data":    map[string]any{"symbol": "BTC-USDT", "price": "100", "quantity": "1", "side": "buy"},
	})

	waitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) >= 1
	})

	mu.Lock()
	msg := got[0]
	mu.Unlock()
	if msg.Channel != "trade" {
		t.Errorf("got channel %q", msg.Channel)
	}
	var trade types.WSTradeEvent
	if err := json.Unmarshal(msg.Data, &trade); err != nil {
		t.Fatalf("decode trade: %v", err)
	}
	if trade.Symbol != "BTC-USDT" || trade.Price != "100" {
		t.Errorf("decoded trade: %+v", trade)
	}

	unsub()
	waitFor(t, 2*time.Second, func() bool {
		return len(ts.requestsByMethod("unsubscribe")) == 1
	})
}

func TestSubscribe_DedupesSharedUpstream(t *testing.T) {
	ts := newTestServer(t)
	client, err := NewClient(fastClientConfig(ts.baseURL()))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	spec := SubscribeSpec{Type: types.WSSubscribeTrade, Symbol: "BTC-USDT"}

	var a, b int32
	unsubA, err := client.Subscribe(context.Background(), spec, func(*types.WSMessage) { atomic.AddInt32(&a, 1) })
	if err != nil {
		t.Fatalf("sub A: %v", err)
	}
	unsubB, err := client.Subscribe(context.Background(), spec, func(*types.WSMessage) { atomic.AddInt32(&b, 1) })
	if err != nil {
		t.Fatalf("sub B: %v", err)
	}

	waitFor(t, time.Second, func() bool { return len(ts.requestsByMethod("subscribe")) >= 1 })

	if got := len(ts.requestsByMethod("subscribe")); got != 1 {
		t.Errorf("expected 1 upstream subscribe, got %d", got)
	}

	conn := ts.latestConn()
	_ = ts.pushNotification(conn, map[string]any{
		"method":  "trade",
		"channel": "trade",
		"data":    map[string]any{"symbol": "BTC-USDT", "price": "1"},
	})

	waitFor(t, time.Second, func() bool {
		return atomic.LoadInt32(&a) >= 1 && atomic.LoadInt32(&b) >= 1
	})

	unsubA()
	time.Sleep(50 * time.Millisecond)
	if got := len(ts.requestsByMethod("unsubscribe")); got != 0 {
		t.Errorf("premature unsubscribe: %d", got)
	}

	unsubB()
	waitFor(t, time.Second, func() bool { return len(ts.requestsByMethod("unsubscribe")) == 1 })
}

func TestFanOut_SymbolMismatchDoesNotDeliver(t *testing.T) {
	ts := newTestServer(t)
	client, err := NewClient(fastClientConfig(ts.baseURL()))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	var delivered int32
	_, err = client.Subscribe(context.Background(),
		SubscribeSpec{Type: types.WSSubscribeTrade, Symbol: "BTC-USDT"},
		func(*types.WSMessage) { atomic.AddInt32(&delivered, 1) })
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitFor(t, time.Second, func() bool { return len(ts.requestsByMethod("subscribe")) == 1 })

	conn := ts.latestConn()
	_ = ts.pushNotification(conn, map[string]any{
		"method":  "trade",
		"channel": "trade",
		"data":    map[string]any{"symbol": "ETH-USDT", "price": "2"},
	})
	_ = ts.pushNotification(conn, map[string]any{
		"method":  "trade",
		"channel": "trade",
		"data":    map[string]any{"symbol": "BTC-USDT", "price": "100"},
	})

	waitFor(t, time.Second, func() bool { return atomic.LoadInt32(&delivered) == 1 })
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&delivered); got != 1 {
		t.Errorf("fan-out delivered to mismatched symbol: %d", got)
	}
}

func TestReconnect_ReplaysSubscriptions(t *testing.T) {
	ts := newTestServer(t)
	cfg := fastClientConfig(ts.baseURL())
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	var received int32
	_, err = client.Subscribe(context.Background(),
		SubscribeSpec{Type: types.WSSubscribeTrade, Symbol: "BTC-USDT"},
		func(*types.WSMessage) { atomic.AddInt32(&received, 1) })
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitFor(t, time.Second, func() bool { return len(ts.requestsByMethod("subscribe")) == 1 })

	ts.dropActiveConnection()

	waitFor(t, 3*time.Second, func() bool { return ts.connCount() >= 2 })
	waitFor(t, 3*time.Second, func() bool { return len(ts.requestsByMethod("subscribe")) >= 2 })

	conn := ts.latestConn()
	_ = ts.pushNotification(conn, map[string]any{
		"method":  "trade",
		"channel": "trade",
		"data":    map[string]any{"symbol": "BTC-USDT", "price": "123"},
	})
	waitFor(t, 2*time.Second, func() bool { return atomic.LoadInt32(&received) >= 1 })
}

func TestBackpressure_DropOldest(t *testing.T) {
	ts := newTestServer(t)
	cfg := fastClientConfig(ts.baseURL())
	cfg.SubscriberBufferSize = 4
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	block := make(chan struct{})
	release := make(chan struct{})
	var seen []string
	var mu sync.Mutex

	_, err = client.Subscribe(context.Background(),
		SubscribeSpec{Type: types.WSSubscribeTrade, Symbol: "BTC-USDT"},
		func(m *types.WSMessage) {
			var payload struct {
				Price string `json:"price"`
			}
			_ = json.Unmarshal(m.Data, &payload)
			mu.Lock()
			first := len(seen) == 0
			seen = append(seen, payload.Price)
			mu.Unlock()
			if first {
				close(block)
				<-release
			}
		})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitFor(t, time.Second, func() bool { return len(ts.requestsByMethod("subscribe")) == 1 })

	conn := ts.latestConn()
	for i := 0; i < 20; i++ {
		_ = ts.pushNotification(conn, map[string]any{
			"method":  "trade",
			"channel": "trade",
			"data":    map[string]any{"symbol": "BTC-USDT", "price": itoa(i)},
		})
	}

	select {
	case <-block:
	case <-time.After(2 * time.Second):
		t.Fatal("handler was never invoked")
	}

	time.Sleep(100 * time.Millisecond)
	close(release)

	waitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(seen) > 1
	})
	mu.Lock()
	total := len(seen)
	mu.Unlock()

	if total >= 20 {
		t.Errorf("expected drop-oldest to discard messages, got %d (no drops)", total)
	}
	if total < 1 {
		t.Errorf("expected some messages to reach handler, got %d", total)
	}
}

func TestSubscribe_ServerReturnsErrorEnvelope(t *testing.T) {
	ts := newTestServer(t)
	ts.mu.Lock()
	ts.ackAll = false
	ts.onRequest = func(conn *websocket.Conn, req types.WSRequest) {
		_ = ts.writeOn(conn, map[string]any{
			"id":     req.ID,
			"status": 400,
			"error":  map[string]any{"code": 42, "message": "bad symbol"},
		})
	}
	ts.mu.Unlock()

	client, err := NewClient(fastClientConfig(ts.baseURL()))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	_, err = client.Subscribe(context.Background(),
		SubscribeSpec{Type: types.WSSubscribeTrade, Symbol: "BOGUS"},
		func(*types.WSMessage) {})
	var wsErr *WSReplyError
	if !errors.As(err, &wsErr) {
		t.Fatalf("want *WSReplyError, got %T: %v", err, err)
	}
	if wsErr.Status != 400 || wsErr.Code != 42 || wsErr.Message != "bad symbol" {
		t.Errorf("unexpected error: %+v", wsErr)
	}
}

func TestClose_UnsubscribesAndStops(t *testing.T) {
	ts := newTestServer(t)
	client, err := NewClient(fastClientConfig(ts.baseURL()))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	var delivered int32
	_, err = client.Subscribe(context.Background(),
		SubscribeSpec{Type: types.WSSubscribeTrade, Symbol: "BTC-USDT"},
		func(*types.WSMessage) { atomic.AddInt32(&delivered, 1) })
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitFor(t, time.Second, func() bool { return len(ts.requestsByMethod("subscribe")) == 1 })

	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err = client.Subscribe(context.Background(),
		SubscribeSpec{Type: types.WSSubscribeTrade, Symbol: "ETH-USDT"},
		func(*types.WSMessage) {})
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Errorf("expected closed error, got %v", err)
	}
}

func TestSubscribe_NilHandler(t *testing.T) {
	client, err := NewClient(fastClientConfig("http://invalid"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if _, err := client.Subscribe(context.Background(),
		SubscribeSpec{Type: "trade", Symbol: "BTC-USDT"}, nil); err == nil {
		t.Error("expected error for nil handler")
	}
}

func TestSubscribe_InvalidSpec(t *testing.T) {
	client, err := NewClient(fastClientConfig("http://invalid"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if _, err := client.Subscribe(context.Background(),
		SubscribeSpec{Symbol: "BTC-USDT"}, func(*types.WSMessage) {}); err == nil {
		t.Error("expected error for missing Type")
	}
	if _, err := client.Subscribe(context.Background(),
		SubscribeSpec{Type: "trade"}, func(*types.WSMessage) {}); err == nil {
		t.Error("expected error for missing Symbol")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var out []byte
	for i > 0 {
		out = append([]byte{byte('0' + i%10)}, out...)
		i /= 10
	}
	return string(out)
}
