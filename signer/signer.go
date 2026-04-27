package signer

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/synthetixio/synthetix-go/eip712"
	"github.com/synthetixio/synthetix-go/types"
)

// DefaultExpiresAfter is the default validity window applied when a
// caller passes 0 for expiresAfterMs on a Sign<Action>() call. 60s is
// long enough to absorb realistic clock skew without keeping a
// replayable signature around.
const DefaultExpiresAfter = 60 * time.Second

// Signer is a high-level helper that produces fully-signed
// /v1/trade request envelopes from raw inputs. It is the Go
// counterpart to the Python SDK's Signer class
// (synthetix/signing.py): the caller supplies a private key once,
// then every Sign<Action>() method returns a wire-ready envelope
// whose body bytes are byte-identical to the bytes that were
// signed.
//
// The envelope builders mirror the public /v1/trade action surface so
// resttrade.Client can post the result without any further marshalling.
// Read-style actions (getPositions, getOpenOrders, …) reuse the
// generic SubAccountActionRequest envelope; write-style actions return
// the typed request from trade_types.go where one exists, and a generic
// SignedEnvelope for the long tail.
//
// Signer is safe for concurrent use; the only mutable state is the
// nonce counter used by NextNonce.
type Signer struct {
	privateKey *ecdsa.PrivateKey
	wallet     common.Address

	mu        sync.Mutex
	lastNonce int64
}

// New builds a Signer from a hex-encoded secp256k1 private key.
// The "0x" prefix is optional. The returned Signer caches the
// derived wallet address so callers can read it without recomputing
// the keccak hash on every use.
func New(privateKeyHex string) (*Signer, error) {
	key := strings.TrimSpace(privateKeyHex)
	if key == "" {
		return nil, errors.New("signer: private key is required")
	}
	key = strings.TrimPrefix(key, "0x")
	pk, err := crypto.HexToECDSA(key)
	if err != nil {
		return nil, fmt.Errorf("signer: parse private key: %w", err)
	}
	return NewFromKey(pk), nil
}

// NewFromKey builds a Signer from an already-parsed *ecdsa.PrivateKey.
// Useful for tests and for callers that load keys from KMS / HSM
// adapters that expose the raw key material.
func NewFromKey(privateKey *ecdsa.PrivateKey) *Signer {
	return &Signer{
		privateKey: privateKey,
		wallet:     crypto.PubkeyToAddress(privateKey.PublicKey),
	}
}

// WalletAddress returns the 0x-prefixed checksum address derived
// from the signer's private key. This is the address the API expects in
// the walletAddress field of every signed envelope.
func (s *Signer) WalletAddress() string {
	return s.wallet.Hex()
}

// PrivateKey returns the underlying ECDSA private key. Exposed for
// callers that need to do additional ad-hoc signing
// (e.g. WebSocket auth, custom integrations) using the same key
// material the Signer was constructed with.
func (s *Signer) PrivateKey() *ecdsa.PrivateKey {
	return s.privateKey
}

// NextNonce returns a strictly-monotonic millisecond-resolution
// nonce. The API deduplicates on (walletAddress, nonce) per
// subaccount, so callers must not reuse a nonce across
// in-flight requests. NextNonce is concurrency-safe and guarantees
// that any two calls return distinct values even if invoked in the
// same millisecond.
func (s *Signer) NextNonce() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	if now <= s.lastNonce {
		now = s.lastNonce + 1
	}
	s.lastNonce = now
	return uint64(now)
}

// resolveExpiry returns the absolute millisecond deadline an action's
// signature is valid until. The public API expects expiresAfter to be
// greater than the millisecond nonce, so callers provide a validity
// window in milliseconds.
func resolveExpiry(nonce uint64, expiresAfterMs int64) int64 {
	if expiresAfterMs <= 0 {
		expiresAfterMs = int64(DefaultExpiresAfter / time.Millisecond)
	}
	return int64(nonce) + expiresAfterMs
}

// signTypedData hashes typedData and returns the SignatureComponents
// suitable for embedding on the wire. Internal helper used by every
// Sign<Action>() method.
func (s *Signer) signTypedData(typedData eip712.TypedData) (types.SignatureComponents, error) {
	hash, err := eip712.Digest(typedData)
	if err != nil {
		return types.SignatureComponents{}, fmt.Errorf("signer: hash typed data: %w", err)
	}
	raw, err := Sign(s.privateKey, hash)
	if err != nil {
		return types.SignatureComponents{}, fmt.Errorf("signer: sign typed data: %w", err)
	}
	sig, err := Split(raw)
	if err != nil {
		return types.SignatureComponents{}, fmt.Errorf("signer: split signature: %w", err)
	}
	return types.SignatureComponents{
		V: int(sig.V),
		R: sig.R,
		S: sig.S,
	}, nil
}

// uintStr formats a uint64 as the decimal string the API
// expects on the wire for subaccountId, nonce, etc.
func uintStr(v uint64) string { return strconv.FormatUint(v, 10) }
