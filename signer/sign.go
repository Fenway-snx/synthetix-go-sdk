package signer

import (
	"crypto/ecdsa"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/synthetixio/synthetix-go/eip712"
)

// Signs a 32-byte digest (typically an EIP-712 message hash) with the
// supplied secp256k1 private key. Returns the raw 65-byte signature
// (r || s || v) suitable for passing to Split.
func Sign(privKey *ecdsa.PrivateKey, digest []byte) ([]byte, error) {
	if privKey == nil {
		return nil, errors.New("signer: private key is required")
	}
	if len(digest) != 32 {
		return nil, errors.New("signer: digest must be 32 bytes")
	}
	return crypto.Sign(digest, privKey)
}

// SignAndSplit is a convenience wrapper that signs a digest and
// returns the wire-shape triple in one call.
func SignAndSplit(privKey *ecdsa.PrivateKey, digest []byte) (Signature, error) {
	sig, err := Sign(privKey, digest)
	if err != nil {
		return Signature{}, err
	}
	return Split(sig)
}

// SignTypedData hashes typedData with EIP-712 and signs the digest
// with privKey. Returns the raw 65-byte signature (r || s || v) with
// v normalised to the {27, 28} range every Synthetix verifier expects.
// Use this when you need the hex-encoded `signature` field for the
// authenticate handshake or any other off-chain endpoint that
// decodes the signature directly with crypto.Ecrecover.
func SignTypedData(privKey *ecdsa.PrivateKey, typedData eip712.TypedData) ([]byte, error) {
	hash, err := eip712.Digest(typedData)
	if err != nil {
		return nil, fmt.Errorf("signer: hash typed data: %w", err)
	}
	raw, err := Sign(privKey, hash)
	if err != nil {
		return nil, err
	}
	if raw[64] < 27 {
		raw[64] += 27
	}
	return raw, nil
}

// SignTypedDataAndSplit is SignTypedData + Split in one call,
// returning the wire-shape (r, s, v) triple that every /v1/trade
// signed envelope embeds.
func SignTypedDataAndSplit(privKey *ecdsa.PrivateKey, typedData eip712.TypedData) (Signature, error) {
	raw, err := SignTypedData(privKey, typedData)
	if err != nil {
		return Signature{}, err
	}
	return Split(raw)
}
