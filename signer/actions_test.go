package signer_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/synthetixio/synthetix-go/eip712"
	"github.com/synthetixio/synthetix-go/signer"
	"github.com/synthetixio/synthetix-go/types"
)

// newTestSigner returns a deterministic Signer constructed from a
// fresh random private key. We never need a fixed key here because
// the recovery test below derives the wallet from the same key the
// Signer was built with.
func newTestSigner(t *testing.T) *signer.Signer {
	t.Helper()
	pk, err := crypto.GenerateKey()
	require.NoError(t, err)
	return signer.NewFromKey(pk)
}

// recoverableV maps the legacy 27/28 V back into the 0/1 form
// crypto.SigToPub expects. Mirrors the inverse of signer.Split.
func reassembleSig(sig types.SignatureComponents) []byte {
	r := common.FromHex(sig.R)
	s := common.FromHex(sig.S)
	v := byte(sig.V)
	if v >= 27 {
		v -= 27
	}
	out := make([]byte, 65)
	copy(out[0:32], r)
	copy(out[32:64], s)
	out[64] = v
	return out
}

// recoverWallet runs ECDSA recovery against the typed-data digest
// and returns the EOA address that produced the signature.
func recoverWallet(t *testing.T, typedData eip712.TypedData, sig types.SignatureComponents) common.Address {
	t.Helper()
	digest, err := eip712.Digest(typedData)
	require.NoError(t, err)
	pub, err := crypto.SigToPub(digest, reassembleSig(sig))
	require.NoError(t, err)
	return crypto.PubkeyToAddress(*pub)
}

func TestNew_AcceptsHexWithAndWithout0xPrefix(t *testing.T) {
	pk, err := crypto.GenerateKey()
	require.NoError(t, err)
	hex := common.Bytes2Hex(crypto.FromECDSA(pk))

	a, err := signer.New(hex)
	require.NoError(t, err)
	b, err := signer.New("0x" + hex)
	require.NoError(t, err)
	require.Equal(t, a.WalletAddress(), b.WalletAddress())
}

func TestNew_RejectsEmpty(t *testing.T) {
	_, err := signer.New("")
	require.Error(t, err)
}

func TestNew_RejectsGarbage(t *testing.T) {
	_, err := signer.New("not-a-hex-key")
	require.Error(t, err)
}

func TestWalletAddress_MatchesECDSAderivation(t *testing.T) {
	pk, err := crypto.GenerateKey()
	require.NoError(t, err)
	s := signer.NewFromKey(pk)
	require.Equal(t, crypto.PubkeyToAddress(pk.PublicKey).Hex(), s.WalletAddress())
}

func TestNextNonce_IsStrictlyMonotonic(t *testing.T) {
	s := newTestSigner(t)
	const N = 1000
	prev := uint64(0)
	for i := 0; i < N; i++ {
		n := s.NextNonce()
		require.Greater(t, n, prev, "nonce must be strictly monotonic")
		prev = n
	}
}

func TestSignPlaceOrders_WireFormatAndRecovery(t *testing.T) {
	s := newTestSigner(t)
	subAccountID := uint64(42)

	req, err := s.SignPlaceOrders(subAccountID, []signer.PlaceOrderInput{{
		Symbol:          "ETH-USDT",
		Side:            "buy",
		OrderType:       "twap",
		Price:           "100.5",
		Quantity:        "1.25",
		PostOnly:        true,
		DurationSeconds: 3600,
		IntervalSeconds: 300,
	}}, "na", 0, 0)
	require.NoError(t, err)
	require.NotNil(t, req)

	require.Equal(t, "42", req.SubAccountID)
	require.Equal(t, s.WalletAddress(), req.WalletAddress)
	require.Greater(t, req.Nonce, uint64(0))
	require.Equal(t, int64(req.Nonce)+int64(signer.DefaultExpiresAfter/time.Millisecond), req.ExpiresAfter)

	// signature recovery must yield the signer's wallet
	typedData := eip712.BuildPlaceOrders(subAccountID, []eip712.PlaceOrderItem{{
		Symbol:    "ETH-USDT",
		Side:      "buy",
		OrderType: "twap",
		Price:     "100.5",
		Quantity:  "1.25",
	}}, "na", req.Nonce, req.ExpiresAfter)
	got := recoverWallet(t, typedData, req.Signature)
	require.Equal(t, common.HexToAddress(s.WalletAddress()), got)

	// JSON envelope round-trips and contains an action under params
	body, err := json.Marshal(req)
	require.NoError(t, err)
	require.Contains(t, string(body), `"action":"placeOrders"`)
	require.Contains(t, string(body), `"walletAddress":"`+s.WalletAddress()+`"`)
	require.Contains(t, string(body), `"postOnly":true`)
	require.Contains(t, string(body), `"durationSeconds":3600`)
	require.Contains(t, string(body), `"intervalSeconds":300`)
}

func TestSignCancelOrders_RecoversAndUsesDecimalIDs(t *testing.T) {
	s := newTestSigner(t)
	req, err := s.SignCancelOrders(7, []uint64{11, 22, 33}, 0, 0)
	require.NoError(t, err)

	td := eip712.BuildCancelOrders(7, []uint64{11, 22, 33}, req.Nonce, req.ExpiresAfter)
	require.Equal(t, common.HexToAddress(s.WalletAddress()), recoverWallet(t, td, req.Signature))

	body, err := json.Marshal(req)
	require.NoError(t, err)
	require.Contains(t, string(body), `"orderIds":["11","22","33"]`)
}

func TestSignCancelAllOrders_NilSymbolsTreatedAsEmpty(t *testing.T) {
	s := newTestSigner(t)
	req, err := s.SignCancelAllOrders(7, nil, 0, 0)
	require.NoError(t, err)

	body, err := json.Marshal(req)
	require.NoError(t, err)
	// nil should serialize as []  — never null
	require.Contains(t, string(body), `"symbols":[]`)
}

func TestSignModifyOrder_OmitsEmptyFields(t *testing.T) {
	s := newTestSigner(t)
	req, err := s.SignModifyOrder(9, 123, "110", "", "", 0, 0)
	require.NoError(t, err)

	td := eip712.BuildModifyOrder(9, 123, "110", "", "", req.Nonce, req.ExpiresAfter)
	require.Equal(t, common.HexToAddress(s.WalletAddress()), recoverWallet(t, td, req.Signature))

	body, err := json.Marshal(req)
	require.NoError(t, err)
	require.Contains(t, string(body), `"price":"110"`)
	require.NotContains(t, string(body), `"quantity":""`)
	require.NotContains(t, string(body), `"triggerPrice":""`)
}

func TestSignWithdrawCollateral_RecoversAndCarriesAllFields(t *testing.T) {
	s := newTestSigner(t)
	dest := "0xAbCdEf0123456789AbCdEf0123456789AbCdEf01"
	req, err := s.SignWithdrawCollateral(9, "USDT", "100", dest, 0, 0)
	require.NoError(t, err)

	td := eip712.BuildWithdrawCollateral(9, "USDT", "100", dest, req.Nonce, req.ExpiresAfter)
	require.Equal(t, common.HexToAddress(s.WalletAddress()), recoverWallet(t, td, req.Signature))

	body, err := json.Marshal(req)
	require.NoError(t, err)
	for _, want := range []string{`"action":"withdrawCollateral"`, `"symbol":"USDT"`, `"amount":"100"`, `"destination":"` + dest + `"`} {
		require.Contains(t, string(body), want)
	}
}

func TestSignTransferCollateral_RecoversAndUsesDecimalToFrom(t *testing.T) {
	s := newTestSigner(t)
	req, err := s.SignTransferCollateral(3, 8, "USDT", "250", 0, 0)
	require.NoError(t, err)

	td := eip712.BuildTransferCollateral(3, 8, "USDT", "250", req.Nonce, req.ExpiresAfter)
	require.Equal(t, common.HexToAddress(s.WalletAddress()), recoverWallet(t, td, req.Signature))

	body, err := json.Marshal(req)
	require.NoError(t, err)
	require.Contains(t, string(body), `"subaccountId":"3"`)
	require.Contains(t, string(body), `"to":"8"`)
}

func TestSignSubAccountAction_NonceZeroDefaultsToCanonicalNonce(t *testing.T) {
	s := newTestSigner(t)
	req, err := s.SignSubAccountAction(3, "getOpenOrders", 0, 0)
	require.NoError(t, err)

	require.Greater(t, req.Nonce, uint64(0))
	require.Equal(t, int64(req.Nonce)+int64(signer.DefaultExpiresAfter/time.Millisecond), req.ExpiresAfter)

	td := eip712.BuildSubAccountAction(3, "getOpenOrders", req.Nonce, req.ExpiresAfter)
	require.Equal(t, common.HexToAddress(s.WalletAddress()), recoverWallet(t, td, req.Signature))

	body, err := json.Marshal(req)
	require.NoError(t, err)
	require.Contains(t, string(body), `"nonce":`)
	require.Contains(t, string(body), `"action":"getOpenOrders"`)
}

func TestSignAuthMessage_RecoversAndDefaultsTimestampToNow(t *testing.T) {
	s := newTestSigner(t)
	sig, ts, err := s.SignAuthMessage(7, 0)
	require.NoError(t, err)
	require.Greater(t, ts, int64(0))

	td := eip712.BuildAuthMessage(7, ts, eip712.ActionWebSocketAuth)
	require.Equal(t, common.HexToAddress(s.WalletAddress()), recoverWallet(t, td, sig))
}

func TestSignAddDelegatedSigner_PermissionsNilBecomesEmpty(t *testing.T) {
	s := newTestSigner(t)
	req, err := s.SignAddDelegatedSigner(3, "0xAbCdEf0123456789AbCdEf0123456789AbCdEf01", nil, 1900000000, 0, 0)
	require.NoError(t, err)

	body, err := json.Marshal(req)
	require.NoError(t, err)
	require.Contains(t, string(body), `"permissions":[]`)
}

func TestSignedRequest_ExpiresAfterReusedAcrossSigningAndPosting(t *testing.T) {
	// Regression: callers must be able to recompute the digest from the
	// envelope's Nonce + ExpiresAfter alone, i.e. the Signer must NOT
	// stash a different value internally that's later used to verify
	// the signature.
	s := newTestSigner(t)
	req, err := s.SignSubAccountAction(3, "getPositions", 555, 999)
	require.NoError(t, err)

	require.Equal(t, uint64(555), req.Nonce)
	require.Equal(t, int64(1554), req.ExpiresAfter)

	td := eip712.BuildSubAccountAction(3, "getPositions", req.Nonce, req.ExpiresAfter)
	require.Equal(t, common.HexToAddress(s.WalletAddress()), recoverWallet(t, td, req.Signature))
}

func TestNonceFormatting_UsesDecimalNotHex(t *testing.T) {
	s := newTestSigner(t)
	req, err := s.SignCancelOrders(7, []uint64{1}, 0, 0)
	require.NoError(t, err)

	body, err := json.Marshal(req)
	require.NoError(t, err)
	require.Contains(t, string(body), `"nonce":`+strconv.FormatUint(req.Nonce, 10))
	require.NotContains(t, string(body), `"nonce":"0x`)
	require.True(t, strings.HasPrefix(req.WalletAddress, "0x"))
}
