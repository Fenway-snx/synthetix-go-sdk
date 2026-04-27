package synthetix_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/Fenway-snx/synthetix-go-sdk/synthetix"
)

func TestNewClient_DefaultsBaseURL(t *testing.T) {
	c, err := synthetix.NewClient(synthetix.Config{})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if c.Info() == nil || c.Trade() == nil || c.WS() == nil {
		t.Fatalf("expected non-nil sub-clients")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestNewClient_NilLoggerIsAccepted(t *testing.T) {
	if _, err := synthetix.NewClient(synthetix.Config{
		BaseURL: "https://api.synthetix.io",
		Logger:  nil,
	}); err != nil {
		t.Fatalf("nil logger should be accepted, got: %v", err)
	}
}

func TestNewClient_NoPrivateKey_HasSignerFalse(t *testing.T) {
	c, err := synthetix.NewClient(synthetix.Config{})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if c.HasSigner() {
		t.Fatalf("expected no signer when PrivateKeyHex is empty")
	}
	if c.Signer() != nil {
		t.Fatalf("expected Signer() == nil")
	}
	if _, err := c.WalletAddress(); !errors.Is(err, synthetix.ErrNoSigner) {
		t.Fatalf("expected ErrNoSigner, got %v", err)
	}
}

func TestNewClient_WithPrivateKey_AttachesSigner(t *testing.T) {
	pk, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	hex := common.Bytes2Hex(crypto.FromECDSA(pk))
	want := crypto.PubkeyToAddress(pk.PublicKey).Hex()

	c, err := synthetix.NewClient(synthetix.Config{PrivateKeyHex: hex})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if !c.HasSigner() {
		t.Fatalf("expected signer to be attached")
	}
	got, err := c.WalletAddress()
	if err != nil {
		t.Fatalf("wallet: %v", err)
	}
	if got != want {
		t.Fatalf("wallet mismatch: got %s, want %s", got, want)
	}
}

func TestNewClient_BadPrivateKeyRejected(t *testing.T) {
	_, err := synthetix.NewClient(synthetix.Config{PrivateKeyHex: "not-a-key"})
	if err == nil {
		t.Fatalf("expected error for malformed private key")
	}
}

func TestClient_NewSigner_LazyAttach(t *testing.T) {
	c, err := synthetix.NewClient(synthetix.Config{})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	pk, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	hex := common.Bytes2Hex(crypto.FromECDSA(pk))

	if err := c.NewSigner(hex); err != nil {
		t.Fatalf("attach signer: %v", err)
	}
	if !c.HasSigner() {
		t.Fatalf("expected signer to be attached")
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv(synthetix.EnvBaseURL, "https://example.test")
	t.Setenv(synthetix.EnvPrivateKey, "0xabc")
	t.Setenv(synthetix.EnvSubAccountID, "42")
	t.Setenv(synthetix.EnvExpiresAfterMs, "120000")

	cfg, err := synthetix.ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if cfg.BaseURL != "https://example.test" || cfg.PrivateKeyHex != "0xabc" || cfg.SubAccountID != 42 || cfg.ExpiresAfterMs != 120000 {
		t.Fatalf("cfg: %+v", cfg)
	}
}

func TestNewReadOnlyClient(t *testing.T) {
	c, err := synthetix.NewReadOnlyClient("https://api.synthetix.io")
	if err != nil {
		t.Fatalf("NewReadOnlyClient: %v", err)
	}
	defer c.Close()
	if c.AuthMode() != synthetix.AuthModeReadOnly {
		t.Fatalf("mode %s", c.AuthMode())
	}
}

func TestClientGetExchangeStatusCombinesRESTAndWS(t *testing.T) {
	statusHandler := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			wantPath := "/v1/exchange/status"
			if name == "ws" {
				wantPath = "/v1/ws/exchange/status"
			}
			if r.URL.Path != wantPath {
				http.NotFound(w, r)
				return
			}
			if r.Method != http.MethodGet {
				t.Fatalf("%s method %s", name, r.Method)
			}
			_, _ = io.WriteString(w, `{"accepting_orders":true,"exchange_status":"RUNNING","code":"","message":"OK","timestamp_ms":1700000000000}`)
		}
	}
	restSrv := httptest.NewServer(statusHandler("rest"))
	defer restSrv.Close()
	wsSrv := httptest.NewServer(statusHandler("ws"))
	defer wsSrv.Close()

	c, err := synthetix.NewClient(synthetix.Config{
		BaseURL:   restSrv.URL,
		WSInfoURL: wsSrv.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	status, err := c.GetExchangeStatus(context.Background())
	if err != nil {
		t.Fatalf("GetExchangeStatus: %v", err)
	}
	if !status.IsRunning() {
		t.Fatalf("status not running: %+v", status)
	}
}

func TestNewTradingClientDiscoversSubAccount(t *testing.T) {
	pk, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	hex := common.Bytes2Hex(crypto.FromECDSA(pk))
	wallet := crypto.PubkeyToAddress(pk.PublicKey).Hex()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/info" {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if req["action"] != "getSubAccountIds" || req["walletAddress"] != wallet {
			t.Fatalf("unexpected request %+v", req)
		}
		_, _ = io.WriteString(w, `{"requestId":"r","response":["7"]}`)
	}))
	defer srv.Close()

	c, err := synthetix.NewTradingClient(context.Background(), synthetix.Config{
		BaseURL:       srv.URL,
		PrivateKeyHex: hex,
	})
	if err != nil {
		t.Fatalf("NewTradingClient: %v", err)
	}
	defer c.Close()
	if id, ok := c.DefaultSubAccountID(); !ok || id != 7 {
		t.Fatalf("default subaccount id %d ok=%v", id, ok)
	}
	status := c.AuthStatus()
	if !status.Ready || status.WalletAddress != wallet || status.SubAccountID != 7 {
		t.Fatalf("status %+v", status)
	}
}

func TestValidateAuthReportsMissingSigner(t *testing.T) {
	c, err := synthetix.NewClient(synthetix.Config{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()
	status, err := c.ValidateAuth(context.Background())
	if err != nil {
		t.Fatalf("ValidateAuth: %v", err)
	}
	if status.Ready || status.HasSigner || len(status.Issues) == 0 {
		t.Fatalf("status %+v", status)
	}
}
