package resttrade

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Fenway-snx/synthetix-go-sdk/types"
)

// testServer spins up an httptest.Server that expects POSTs to
// /v1/trade and dispatches by action. responder returns (statusCode,
// responseBody) for a given decoded request body.
func testServer(t *testing.T, responder func(t *testing.T, req map[string]any) (int, string)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/trade" {
			t.Errorf("expected /v1/trade, got %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		if ua := r.Header.Get("User-Agent"); ua != DefaultUserAgent {
			t.Errorf("User-Agent %q", ua)
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

	client, err := NewClient(Config{BaseURL: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// sampleSignature returns a deterministic signature stub for request
// envelopes. Its contents are never inspected by resttrade (the
// signing boundary lives upstream), only echoed to the test server.
func sampleSignature() types.SignatureComponents {
	return types.SignatureComponents{
		V: 27,
		R: "0x0000000000000000000000000000000000000000000000000000000000000001",
		S: "0x0000000000000000000000000000000000000000000000000000000000000002",
	}
}

// paramsOf extracts the "params" map from a decoded request body for
// assertion convenience.
func paramsOf(t *testing.T, req map[string]any) map[string]any {
	t.Helper()
	p, ok := req["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected params object, got %T", req["params"])
	}
	return p
}

func TestNewClient_RequiresBaseURL(t *testing.T) {
	if _, err := NewClient(Config{}); err == nil {
		t.Fatal("expected error for empty BaseURL")
	}
}

func TestPlaceOrders_WireShapeAndResponse(t *testing.T) {
	client := testServer(t, func(t *testing.T, req map[string]any) (int, string) {
		p := paramsOf(t, req)
		if p["action"] != "placeOrders" {
			t.Errorf("action %v", p["action"])
		}
		if p["subaccountId"] != "42" {
			t.Errorf("subaccountId %v", p["subaccountId"])
		}
		if req["nonce"].(float64) != 7 {
			t.Errorf("nonce %v", req["nonce"])
		}
		sig := req["signature"].(map[string]any)
		if sig["v"].(float64) != 27 {
			t.Errorf("sig.v %v", sig["v"])
		}
		return 200, `{"requestId":"r","response":{"statuses":[{"resting":{"order":{"venueId":"123"},"id":"123"}}]}}`
	})

	resp, err := client.PlaceOrders(context.Background(), &types.PlaceOrdersRequest{
		Params: types.PlaceOrdersAction{
			Action:       "placeOrders",
			SubAccountID: "42",
			Orders: []types.PlaceOrderItem{
				{Symbol: "BTC-USDT", Side: "BUY", OrderType: "LIMIT", Price: "100", Quantity: "1"},
			},
		},
		SubAccountID:  "42",
		WalletAddress: "0xabc",
		Nonce:         7,
		Signature:     sampleSignature(),
	})
	if err != nil {
		t.Fatalf("PlaceOrders: %v", err)
	}
	if len(resp.Statuses) != 1 || resp.Statuses[0].Resting == nil || resp.Statuses[0].Resting.OrderID.VenueID != "123" {
		t.Errorf("statuses: %+v", resp.Statuses)
	}
}

// Note: a previous TestPlaceOrders_AutoFillsAction asserted that the
// resttrade client would silently populate a missing `action` field
// on the Params struct. The envelope must be byte-identical to what
// was EIP-712 signed, and post-sign mutation breaks that invariant.
// Callers now set action explicitly.

func TestModifyOrder(t *testing.T) {
	client := testServer(t, func(t *testing.T, req map[string]any) (int, string) {
		p := paramsOf(t, req)
		if p["action"] != "modifyOrder" {
			t.Errorf("action %v", p["action"])
		}
		if p["orderId"] != "101" {
			t.Errorf("orderId %v", p["orderId"])
		}
		if _, has := p["clientOrderId"]; has {
			t.Error("clientOrderId should be omitted when empty")
		}
		return 200, `{"requestId":"r","response":{"order":{"venueId":"101","clientId":""},"orderId":"101","status":"NEW","price":"100","quantity":"2","timestamp":1700000000000}}`
	})
	resp, err := client.ModifyOrder(context.Background(), &types.ModifyOrderRequest{
		Params:        types.ModifyOrderAction{Action: "modifyOrder", SubAccountID: "1", OrderID: "101", Price: "100", Quantity: "2"},
		SubAccountID:  "1",
		WalletAddress: "0xabc",
		Nonce:         1,
		Signature:     sampleSignature(),
	})
	if err != nil {
		t.Fatalf("ModifyOrder: %v", err)
	}
	if resp.Order.VenueID != "101" || resp.DeprecatedVenueID != "101" || resp.Price != "100" || resp.Status != "NEW" {
		t.Errorf("%+v", resp)
	}
}

func TestCancelOrders(t *testing.T) {
	client := testServer(t, func(t *testing.T, req map[string]any) (int, string) {
		p := paramsOf(t, req)
		ids, _ := p["orderIds"].([]any)
		if len(ids) != 2 {
			t.Errorf("orderIds %v", ids)
		}
		return 200, `{"requestId":"r","response":{"statuses":[{"canceled":{"order":{"venueId":"1"},"id":"1"}},{"error":"not_found","errorCode":"NOT_FOUND","order":{"order":{"venueId":"2"},"id":"2"}}]}}`
	})
	resp, err := client.CancelOrders(context.Background(), &types.CancelOrdersRequest{
		Params:        types.CancelOrdersAction{Action: "cancelOrders", SubAccountID: "1", OrderIDs: []string{"1", "2"}},
		SubAccountID:  "1",
		WalletAddress: "0xabc",
		Nonce:         1,
		Signature:     sampleSignature(),
	})
	if err != nil {
		t.Fatalf("CancelOrders: %v", err)
	}
	if len(resp.Statuses) != 2 ||
		resp.Statuses[0].Canceled == nil || resp.Statuses[0].Canceled.OrderID.VenueID != "1" ||
		resp.Statuses[1].Error != "not_found" || resp.Statuses[1].ErrorCode != "NOT_FOUND" {
		t.Errorf("%+v", resp)
	}
}

func TestCancelAllOrders(t *testing.T) {
	client := testServer(t, func(t *testing.T, req map[string]any) (int, string) {
		p := paramsOf(t, req)
		if p["action"] != "cancelAllOrders" {
			t.Errorf("action %v", p["action"])
		}
		return 200, `{"requestId":"r","response":[{"order":{"venueId":"11"},"orderId":"11","message":"","symbol":"BTC-USDT"},{"order":{"venueId":"12"},"orderId":"12","message":"","symbol":"ETH-USDT"}]}`
	})
	resp, err := client.CancelAllOrders(context.Background(), &types.CancelAllOrdersRequest{
		Params:        types.CancelAllOrdersAction{Action: "cancelAllOrders", SubAccountID: "1"},
		SubAccountID:  "1",
		WalletAddress: "0xabc",
		Nonce:         1,
		Signature:     sampleSignature(),
	})
	if err != nil {
		t.Fatalf("CancelAllOrders: %v", err)
	}
	if len(*resp) != 2 || (*resp)[0].Order.VenueID != "11" || (*resp)[1].Order.VenueID != "12" {
		t.Errorf("%+v", resp)
	}
}

func TestGetSubAccount_ReturnsSingle(t *testing.T) {
	client := testServer(t, func(t *testing.T, req map[string]any) (int, string) {
		if req["walletAddress"] != "0xabc" {
			t.Errorf("walletAddress %v", req["walletAddress"])
		}
		return 200, `{"requestId":"r","response":{"subAccountId":"1","subAccountName":"Master","crossMarginSummary":{"accountValue":"1000","availableMargin":"900","totalUnrealizedPnl":"10","maintenanceMargin":"50","initialMargin":"100","withdrawable":"800","adjustedAccountValue":"1000","debt":"0"},"collaterals":[],"positions":[],"marketPreferences":{"leverages":{}},"feeRates":{"makerFeeRate":"0.0002","takerFeeRate":"0.0005","tierName":"Regular User"},"accountLimits":{"maxBorrowCapacity":"5000","maxOrdersPerMarket":10,"maxSubAccounts":1,"maxTotalOrders":50}}}`
	})
	out, err := client.GetSubAccount(context.Background(), &types.SubAccountActionRequest{
		SubAccountID:  "1",
		WalletAddress: "0xabc",
		Signature:     sampleSignature(),
	})
	if err != nil {
		t.Fatalf("GetSubAccount: %v", err)
	}
	if out == nil {
		t.Fatalf("GetSubAccount returned nil response")
	}
	if out.MarginSummary.AccountValue != "1000" {
		t.Errorf("accountValue %q", out.MarginSummary.AccountValue)
	}
	if out.FeeRates.TierName != "Regular User" {
		t.Errorf("tierName %q", out.FeeRates.TierName)
	}
}

func TestGetSubAccounts_UnwrapsEnvelope(t *testing.T) {
	client := testServer(t, func(t *testing.T, _ map[string]any) (int, string) {
		return 200, `{"requestId":"r","response":{"subAccounts":[{"subAccountId":"1","subAccountName":"Master","crossMarginSummary":{"accountValue":"0","availableMargin":"0","totalUnrealizedPnl":"0","maintenanceMargin":"0","initialMargin":"0","withdrawable":"0","adjustedAccountValue":"0","debt":"0"},"collaterals":[],"positions":[],"marketPreferences":{"leverages":{}},"feeRates":{"makerFeeRate":"","takerFeeRate":"","tierName":""},"accountLimits":{"maxBorrowCapacity":"","maxOrdersPerMarket":0,"maxSubAccounts":0,"maxTotalOrders":0},"delegatedSigners":[{"subAccountId":"1","walletAddress":"0xd","permissions":["trading"],"expiresAt":null}]}]}}`
	})
	out, err := client.GetSubAccounts(context.Background(), &types.SubAccountActionRequest{
		SubAccountID:  "1",
		WalletAddress: "0xabc",
		Signature:     sampleSignature(),
	})
	if err != nil {
		t.Fatalf("GetSubAccounts: %v", err)
	}
	if len(out) != 1 || len(out[0].DelegatedSigners) != 1 || out[0].DelegatedSigners[0].Permissions[0] != "trading" {
		t.Errorf("%+v", out)
	}
}

func TestGetOpenOrders(t *testing.T) {
	client := testServer(t, func(t *testing.T, _ map[string]any) (int, string) {
		return 200, `{"requestId":"r","response":[{"order":{"venueId":"101","clientId":"cli-a"},"orderId":"101","symbol":"BTC-USDT","side":"BUY","type":"LIMIT","price":"100","quantity":"1","filledQuantity":"0","timeInForce":"GTC","createdTime":1700000000000,"updatedTime":1700000000001,"reduceOnly":false,"postOnly":false,"closePosition":false,"triggerPrice":"0","triggerPriceType":""}]}`
	})
	out, err := client.GetOpenOrders(context.Background(), &types.SubAccountActionRequest{
		SubAccountID:  "1",
		WalletAddress: "0xabc",
		Signature:     sampleSignature(),
	})
	if err != nil {
		t.Fatalf("GetOpenOrders: %v", err)
	}
	if len(out) != 1 || out[0].Order.VenueID != "101" || out[0].Order.ClientID != "cli-a" {
		t.Errorf("%+v", out)
	}
	if out[0].Type != "LIMIT" || out[0].TimeInForce != "GTC" {
		t.Errorf("type/tif %+v", out[0])
	}
}

func TestGetPositions(t *testing.T) {
	client := testServer(t, func(t *testing.T, _ map[string]any) (int, string) {
		return 200, `{"requestId":"r","response":[{"positionId":"42","subAccountId":"1","symbol":"BTC-USDT","side":"long","quantity":"1","entryPrice":"100","unrealizedPnl":"2","realizedPnl":"0","liquidationPrice":"0","usedMargin":"4","maintenanceMargin":"1","adlBucket":3,"updatedAt":1700000000000,"createdAt":1699999999999}]}`
	})
	out, err := client.GetPositions(context.Background(), &types.SubAccountActionRequest{
		SubAccountID:  "1",
		WalletAddress: "0xabc",
		Signature:     sampleSignature(),
	})
	if err != nil {
		t.Fatalf("GetPositions: %v", err)
	}
	if len(out) != 1 || out[0].Side != "long" || out[0].UnrealizedPnl != "2" || out[0].PositionID != "42" {
		t.Errorf("%+v", out)
	}
}

func TestCreateSubaccount(t *testing.T) {
	client := testServer(t, func(t *testing.T, _ map[string]any) (int, string) {
		return 200, `{"requestId":"r","response":{"subAccountId":"2","subAccountName":"trader"}}`
	})
	out, err := client.CreateSubaccount(context.Background(), &types.CreateSubaccountRequest{
		Params:    types.CreateSubaccountAction{SubAccountID: "1", Name: "trader"},
		Nonce:     1,
		Signature: sampleSignature(),
	})
	if err != nil {
		t.Fatalf("CreateSubaccount: %v", err)
	}
	if out.SubAccountID != "2" || out.SubAccountName != "trader" {
		t.Errorf("%+v", out)
	}
}

func TestUpdateSubAccountName(t *testing.T) {
	client := testServer(t, func(t *testing.T, _ map[string]any) (int, string) {
		return 200, `{"requestId":"r","response":{"subAccountId":"1","name":"renamed"}}`
	})
	out, err := client.UpdateSubAccountName(context.Background(), &types.UpdateSubAccountNameRequest{
		Params:       types.UpdateSubAccountNameAction{Name: "renamed"},
		SubAccountID: "1",
		Nonce:        1,
		Signature:    sampleSignature(),
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out.Name != "renamed" {
		t.Errorf("%+v", out)
	}
}

func TestAddDelegatedSigner(t *testing.T) {
	client := testServer(t, func(t *testing.T, req map[string]any) (int, string) {
		p := paramsOf(t, req)
		perms, _ := p["permissions"].([]any)
		if len(perms) != 1 || perms[0] != "trading" {
			t.Errorf("permissions %v", perms)
		}
		return 200, `{"requestId":"r","response":{"subAccountId":"1","walletAddress":"0xd"}}`
	})
	out, err := client.AddDelegatedSigner(context.Background(), &types.AddDelegatedSignerRequest{
		Params:       types.AddDelegatedSignerAction{WalletAddress: "0xd", Permissions: []string{"trading"}},
		SubAccountID: "1",
		Nonce:        1,
		Signature:    sampleSignature(),
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out.WalletAddress != "0xd" {
		t.Errorf("%+v", out)
	}
}

func TestRemoveAllDelegatedSigners(t *testing.T) {
	client := testServer(t, func(t *testing.T, _ map[string]any) (int, string) {
		return 200, `{"requestId":"r","response":{"subAccountId":"1","removedSigners":["0x1","0x2"]}}`
	})
	out, err := client.RemoveAllDelegatedSigners(context.Background(), &types.RemoveAllDelegatedSignersRequest{
		Params:       types.RemoveAllDelegatedSignersAction{},
		SubAccountID: "1",
		Nonce:        1,
		Signature:    sampleSignature(),
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out.RemovedSigners) != 2 {
		t.Errorf("%+v", out)
	}
}

func TestSendSigned_PassesThroughRawBody(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"requestId":"r","response":{"ok":true}}`)
	}))
	t.Cleanup(srv.Close)
	client, _ := NewClient(Config{BaseURL: srv.URL, HTTPClient: srv.Client()})

	raw := json.RawMessage(`{"params":{"action":"opaque"},"nonce":99,"signature":{"v":27,"r":"0x1","s":"0x2"}}`)
	var out struct {
		Ok bool `json:"ok"`
	}
	if err := client.SendSigned(context.Background(), "opaque", raw, &out); err != nil {
		t.Fatalf("SendSigned: %v", err)
	}
	if !out.Ok {
		t.Error("response not decoded")
	}
	// Body should be forwarded byte-for-byte.
	if string(gotBody) != string(raw) {
		t.Errorf("body mutated: got %s want %s", gotBody, raw)
	}
}

func TestSendSigned_EmptyBodyRejected(t *testing.T) {
	client, _ := NewClient(Config{BaseURL: "https://example.invalid"})
	if err := client.SendSigned(context.Background(), "x", nil, nil); err == nil {
		t.Error("expected error for empty body")
	}
}

func TestRESTError_FromStructuredErrorBranch(t *testing.T) {
	client := testServer(t, func(t *testing.T, _ map[string]any) (int, string) {
		return 400, `{"requestId":"r-err","error":{"code":"NONCE_REUSED","message":"nonce already used"}}`
	})
	_, err := client.PlaceOrders(context.Background(), &types.PlaceOrdersRequest{
		Params:        types.PlaceOrdersAction{Action: "placeOrders", SubAccountID: "1", Orders: []types.PlaceOrderItem{{Symbol: "S", Side: "BUY", OrderType: "LIMIT", Price: "1", Quantity: "1"}}},
		SubAccountID:  "1",
		WalletAddress: "0xabc",
		Nonce:         1,
		Signature:     sampleSignature(),
	})
	var restErr *RESTError
	if !errors.As(err, &restErr) {
		t.Fatalf("want *RESTError, got %T: %v", err, err)
	}
	if restErr.Code != "NONCE_REUSED" || restErr.StatusCode != 400 {
		t.Errorf("unexpected RESTError: %+v", restErr)
	}
}

func TestTransportError_NonJSONBody(t *testing.T) {
	client := testServer(t, func(t *testing.T, _ map[string]any) (int, string) {
		return 502, `<html>bad gateway</html>`
	})
	_, err := client.ModifyOrder(context.Background(), &types.ModifyOrderRequest{
		Params:        types.ModifyOrderAction{Action: "modifyOrder", SubAccountID: "1", OrderID: "1"},
		SubAccountID:  "1",
		WalletAddress: "0xabc",
		Nonce:         1,
		Signature:     sampleSignature(),
	})
	var tErr *TransportError
	if !errors.As(err, &tErr) {
		t.Fatalf("want *TransportError, got %T: %v", err, err)
	}
	if tErr.StatusCode != 502 {
		t.Errorf("status %d", tErr.StatusCode)
	}
}

func TestNilRequestRejected(t *testing.T) {
	client, _ := NewClient(Config{BaseURL: "https://example.invalid"})
	if _, err := client.PlaceOrders(context.Background(), nil); err == nil {
		t.Error("PlaceOrders nil")
	}
	if _, err := client.ModifyOrder(context.Background(), nil); err == nil {
		t.Error("ModifyOrder nil")
	}
	if _, err := client.CancelOrders(context.Background(), nil); err == nil {
		t.Error("CancelOrders nil")
	}
	if _, err := client.CancelAllOrders(context.Background(), nil); err == nil {
		t.Error("CancelAllOrders nil")
	}
	if _, err := client.GetSubAccount(context.Background(), nil); err == nil {
		t.Error("GetSubAccount nil")
	}
	if _, err := client.GetSubAccounts(context.Background(), nil); err == nil {
		t.Error("GetSubAccounts nil")
	}
	if _, err := client.GetOpenOrders(context.Background(), nil); err == nil {
		t.Error("GetOpenOrders nil")
	}
	if _, err := client.GetPositions(context.Background(), nil); err == nil {
		t.Error("GetPositions nil")
	}
	if _, err := client.CreateSubaccount(context.Background(), nil); err == nil {
		t.Error("CreateSubaccount nil")
	}
	if _, err := client.UpdateSubAccountName(context.Background(), nil); err == nil {
		t.Error("UpdateSubAccountName nil")
	}
	if _, err := client.AddDelegatedSigner(context.Background(), nil); err == nil {
		t.Error("AddDelegatedSigner nil")
	}
	if _, err := client.RemoveAllDelegatedSigners(context.Background(), nil); err == nil {
		t.Error("RemoveAllDelegatedSigners nil")
	}
}

func TestAdditionalWriteActions(t *testing.T) {
	tests := []struct {
		name   string
		action string
		call   func(*Client) error
	}{
		{
			name:   "cancel by cloid",
			action: "cancelOrders",
			call: func(c *Client) error {
				_, err := c.CancelOrdersByCloid(context.Background(), &types.CancelOrdersRequest{
					Params:        types.CancelOrdersAction{Action: "cancelOrders", ClientOrderIDs: []string{"cli-1"}},
					SubAccountID:  "1",
					WalletAddress: "0xabc",
					Nonce:         1,
					Signature:     sampleSignature(),
				})
				return err
			},
		},
		{
			name:   "update leverage",
			action: "updateLeverage",
			call: func(c *Client) error {
				_, err := c.UpdateLeverage(context.Background(), &types.UpdateLeverageRequest{
					Params:        map[string]any{"action": "updateLeverage", "symbol": "BTC-USDT", "leverage": "5"},
					SubAccountID:  "1",
					WalletAddress: "0xabc",
					Nonce:         1,
					Signature:     sampleSignature(),
				})
				return err
			},
		},
		{
			name:   "withdraw collateral",
			action: "withdrawCollateral",
			call: func(c *Client) error {
				_, err := c.WithdrawCollateral(context.Background(), &types.WithdrawCollateralRequest{
					Params:        map[string]any{"action": "withdrawCollateral", "symbol": "USDC", "amount": "10", "destination": "0xabc"},
					SubAccountID:  "1",
					WalletAddress: "0xabc",
					Nonce:         1,
					Signature:     sampleSignature(),
				})
				return err
			},
		},
		{
			name:   "transfer collateral",
			action: "transferCollateral",
			call: func(c *Client) error {
				_, err := c.TransferCollateral(context.Background(), &types.TransferCollateralRequest{
					Params:        map[string]any{"action": "transferCollateral", "symbol": "USDC", "amount": "10", "to": "2"},
					SubAccountID:  "1",
					WalletAddress: "0xabc",
					Nonce:         1,
					Signature:     sampleSignature(),
				})
				return err
			},
		},
		{
			name:   "schedule cancel",
			action: "scheduleCancel",
			call: func(c *Client) error {
				_, err := c.ScheduleCancel(context.Background(), &types.ScheduleCancelRequest{
					Params:        map[string]any{"action": "scheduleCancel", "timeoutSeconds": float64(60)},
					SubAccountID:  "1",
					WalletAddress: "0xabc",
					Nonce:         1,
					Signature:     sampleSignature(),
				})
				return err
			},
		},
		{
			name:   "remove delegated signer",
			action: "removeDelegatedSigner",
			call: func(c *Client) error {
				_, err := c.RemoveDelegatedSigner(context.Background(), &types.RemoveDelegatedSignerRequest{
					Params:        map[string]any{"action": "removeDelegatedSigner", "walletAddress": "0xdef"},
					SubAccountID:  "1",
					WalletAddress: "0xabc",
					Nonce:         1,
					Signature:     sampleSignature(),
				})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testServer(t, func(t *testing.T, req map[string]any) (int, string) {
				if action := paramsOf(t, req)["action"]; action != tt.action {
					t.Errorf("action %v", action)
				}
				return 200, `{"requestId":"r","response":{"ok":true}}`
			})
			if err := tt.call(client); err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
		})
	}
}

func TestAdditionalAuthenticatedReads(t *testing.T) {
	tests := []struct {
		name   string
		action string
		call   func(*Client) error
		body   string
	}{
		{"balance updates", "getBalanceUpdates", func(c *Client) error { _, err := c.GetBalanceUpdates(context.Background(), signedRead("getBalanceUpdates")); return err }, `[{"id":"b1"}]`},
		{"transfers", "getTransfers", func(c *Client) error { _, err := c.GetTransfers(context.Background(), signedRead("getTransfers")); return err }, `[{"id":"t1"}]`},
		{"position history", "getPositionHistory", func(c *Client) error { _, err := c.GetPositionHistory(context.Background(), signedRead("getPositionHistory")); return err }, `[{"positionId":"p1"}]`},
		{"portfolio", "getPortfolio", func(c *Client) error { _, err := c.GetPortfolio(context.Background(), signedRead("getPortfolio")); return err }, `{"accountValue":"100"}`},
		{"trades for position", "getTradesForPosition", func(c *Client) error { _, err := c.GetTradesForPosition(context.Background(), signedRead("getTradesForPosition")); return err }, `{"trades":[],"hasMore":false,"total":0}`},
		{"fees", "getFees", func(c *Client) error { _, err := c.GetFees(context.Background(), signedRead("getFees")); return err }, `{"makerFeeRate":"0.0002"}`},
		{"rate limits", "getRateLimits", func(c *Client) error { _, err := c.GetRateLimits(context.Background(), signedRead("getRateLimits")); return err }, `{"remaining":10}`},
		{"delegated signers", "getDelegatedSigners", func(c *Client) error { _, err := c.GetDelegatedSigners(context.Background(), signedRead("getDelegatedSigners")); return err }, `[{"subAccountId":"1","walletAddress":"0xdef","permissions":["trading"],"expiresAt":null}]`},
		{"delegations for delegate", "getDelegationsForDelegate", func(c *Client) error { _, err := c.GetDelegationsForDelegate(context.Background(), signedRead("getDelegationsForDelegate")); return err }, `[{"subAccountId":"1","walletAddress":"0xdef","permissions":["trading"],"expiresAt":null}]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testServer(t, func(t *testing.T, req map[string]any) (int, string) {
				if action := paramsOf(t, req)["action"]; action != tt.action {
					t.Errorf("action %v", action)
				}
				return 200, `{"requestId":"r","response":` + tt.body + `}`
			})
			if err := tt.call(client); err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
		})
	}
}

func signedRead(action string) *types.SubAccountActionRequest {
	return &types.SubAccountActionRequest{
		Params:        map[string]any{"action": action},
		SubAccountID:  "1",
		WalletAddress: "0xabc",
		Signature:     sampleSignature(),
	}
}
