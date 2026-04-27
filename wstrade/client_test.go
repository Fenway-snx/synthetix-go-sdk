package wstrade

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/websocket"

	"github.com/Fenway-snx/synthetix-go-sdk/signer"
	"github.com/Fenway-snx/synthetix-go-sdk/types"
)

func TestToWSURL(t *testing.T) {
	got, err := toWSURL("https://api.synthetix.io")
	if err != nil {
		t.Fatalf("toWSURL: %v", err)
	}
	if got != "wss://api.synthetix.io/v1/ws/trade" {
		t.Fatalf("got %q", got)
	}
}

func TestProtocolAuthPostSubscribeAndNotify(t *testing.T) {
	pk, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	s, err := signer.New(common.Bytes2Hex(crypto.FromECDSA(pk)))
	if err != nil {
		t.Fatalf("signer: %v", err)
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	authDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ws/trade" {
			t.Errorf("path %s", r.URL.Path)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		auth := readWSRequest(t, conn)
		if auth.Method != "auth" {
			t.Errorf("auth request %+v", auth)
		}
		if _, ok := auth.Params["message"].(string); !ok {
			t.Errorf("missing auth message: %+v", auth.Params)
		}
		if _, ok := auth.Params["signature"].(string); !ok {
			t.Errorf("missing auth signature: %+v", auth.Params)
		}
		writeWSReplyRequestID(t, conn, auth.ID, `{"ok":true}`)
		close(authDone)

		post := readWSRequest(t, conn)
		if post.Method != "post" || post.Params["action"] != "cancelOrders" || post.Params["subaccountId"] != "1" {
			t.Errorf("post request %+v", post)
		}
		writeWSReplyRequestID(t, conn, post.ID, `{"response":{"statuses":[]}}`)

		sub := readWSRequest(t, conn)
		if sub.Method != "subscribe" || sub.Params["type"] != "subAccountUpdate" {
			t.Errorf("subscribe request %+v", sub)
		}
		writeWSReply(t, conn, sub.ID, `{"subscribed":true}`)
		if err := conn.WriteJSON(map[string]any{
			"channel":   "subAccountUpdate",
			"timestamp": int64(1700000000000),
			"data": map[string]any{
				"subAccountId": "1",
			},
		}); err != nil {
			t.Errorf("write notification: %v", err)
		}

		// Keep the connection open until the client closes it.
		for {
			var req types.WSRequest
			if err := conn.ReadJSON(&req); err != nil {
				return
			}
			writeWSReply(t, conn, req.ID, `{"ok":true}`)
		}
	}))
	defer srv.Close()

	c, err := NewClient(Config{
		BaseURL:          srv.URL,
		SubAccountID:    1,
		Signer:          s,
		RequestTimeout:  time.Second,
		ReconnectInitial: time.Millisecond,
		PingInterval:    -1,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	select {
	case <-authDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for auth")
	}

	var postOut types.CancelOrdersResponse
	err = c.Post(context.Background(), &types.CancelOrdersRequest{
		Params:        map[string]any{"action": "cancelOrders", "orderIds": []string{"1"}},
		SubAccountID:  "1",
		WalletAddress: s.WalletAddress(),
		Nonce:         1,
		Signature:     types.SignatureComponents{V: 27, R: "0x1", S: "0x2"},
	}, &postOut)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}

	updates, unsubscribe, err := c.SubscribeSubAccountUpdatesChan(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("SubscribeSubAccountUpdatesChan: %v", err)
	}
	defer unsubscribe()

	select {
	case msg := <-updates:
		if msg.Channel != "subAccountUpdate" {
			t.Fatalf("notification channel %q", msg.Channel)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

func TestCloseUnblocksActiveReadLoop(t *testing.T) {
	pk, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	s, err := signer.New(common.Bytes2Hex(crypto.FromECDSA(pk)))
	if err != nil {
		t.Fatalf("signer: %v", err)
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	authDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		auth := readWSRequest(t, conn)
		writeWSReply(t, conn, auth.ID, `{"ok":true}`)
		close(authDone)

		// Keep the websocket open so Close must unblock the client's
		// ReadMessage call rather than waiting for ReadTimeout.
		<-r.Context().Done()
	}))
	defer srv.Close()

	c, err := NewClient(Config{
		BaseURL:          srv.URL,
		SubAccountID:    1,
		Signer:          s,
		ReadTimeout:     time.Hour,
		RequestTimeout:  time.Second,
		ReconnectInitial: time.Millisecond,
		PingInterval:    -1,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	select {
	case <-authDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for auth")
	}

	done := make(chan error, 1)
	go func() { done <- c.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Close waited for websocket read timeout")
	}
}

func TestPostWaitsForAuthenticatedConnection(t *testing.T) {
	pk, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	s, err := signer.New(common.Bytes2Hex(crypto.FromECDSA(pk)))
	if err != nil {
		t.Fatalf("signer: %v", err)
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	authSeen := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		auth := readWSRequest(t, conn)
		close(authSeen)
		writeWSReply(t, conn, auth.ID, `{"ok":true}`)

		post := readWSRequest(t, conn)
		if post.Method != "post" {
			t.Errorf("post method %q", post.Method)
		}
		writeWSReply(t, conn, post.ID, `{"statuses":[]}`)
	}))
	defer srv.Close()

	c, err := NewClient(Config{
		BaseURL:          srv.URL,
		SubAccountID:    1,
		Signer:          s,
		RequestTimeout:  2 * time.Second,
		ReconnectInitial: time.Millisecond,
		PingInterval:    -1,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	var out types.CancelOrdersResponse
	err = c.Post(context.Background(), &types.CancelOrdersRequest{
		Params:        map[string]any{"action": "cancelOrders", "orderIds": []string{"1"}},
		SubAccountID:  "1",
		WalletAddress: s.WalletAddress(),
		Nonce:         1,
		Signature:     types.SignatureComponents{V: 27, R: "0x1", S: "0x2"},
	}, &out)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	select {
	case <-authSeen:
	default:
		t.Fatal("post returned before auth")
	}
}

func readWSRequest(t *testing.T, conn *websocket.Conn) types.WSRequest {
	t.Helper()
	var req types.WSRequest
	if err := conn.ReadJSON(&req); err != nil {
		t.Fatalf("read request: %v", err)
	}
	return req
}

func writeWSReply(t *testing.T, conn *websocket.Conn, id string, result string) {
	t.Helper()
	var raw json.RawMessage = []byte(result)
	if err := conn.WriteJSON(&types.WSMessage{
		ID:     id,
		Status: 200,
		Result: raw,
	}); err != nil {
		t.Fatalf("write reply: %v", err)
	}
}

func writeWSReplyRequestID(t *testing.T, conn *websocket.Conn, id string, result string) {
	t.Helper()
	var raw json.RawMessage = []byte(result)
	if err := conn.WriteJSON(&types.WSMessage{
		RequestID: id,
		Status:    200,
		Result:    raw,
	}); err != nil {
		t.Fatalf("write reply: %v", err)
	}
}

func TestNewClientValidation(t *testing.T) {
	if _, err := NewClient(Config{}); err == nil {
		t.Fatal("expected missing BaseURL error")
	}
	if _, err := NewClient(Config{BaseURL: "https://api.synthetix.io"}); err == nil {
		t.Fatal("expected missing Signer error")
	}

	pk, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	s, err := signer.New(common.Bytes2Hex(crypto.FromECDSA(pk)))
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	if _, err := NewClient(Config{BaseURL: "https://api.synthetix.io", Signer: s}); err == nil {
		t.Fatal("expected missing SubAccountID error")
	}
}

func TestFlattenSignedEnvelope(t *testing.T) {
	params, err := flattenSignedEnvelope(&types.CancelOrdersRequest{
		Params:        map[string]any{"action": "cancelOrders", "clientOrderIds": []string{"cli-1"}},
		SubAccountID:  "1",
		WalletAddress: "0xabc",
		Nonce:         7,
		Signature:     types.SignatureComponents{V: 27, R: "0x1", S: "0x2"},
	})
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}
	if params["action"] != "cancelOrders" || params["subaccountId"] != "1" || params["walletAddress"] != "0xabc" {
		t.Fatalf("params %+v", params)
	}
	if sig, ok := params["signature"].(map[string]any); !ok || sig["r"] != "0x1" {
		t.Fatalf("signature %+v", params["signature"])
	}
}

