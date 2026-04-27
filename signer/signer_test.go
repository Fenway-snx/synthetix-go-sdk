package signer_test

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/Fenway-snx/synthetix-go-sdk/signer"
)

func TestSplit_RejectsShortInput(t *testing.T) {
	if _, err := signer.Split(make([]byte, 64)); err == nil {
		t.Fatalf("expected error for 64-byte input")
	}
}

func TestSplit_NormalisesVTo27Or28(t *testing.T) {
	sig := make([]byte, 65)
	sig[64] = 0
	out, err := signer.Split(sig)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if out.V != 27 {
		t.Fatalf("expected v=27, got %d", out.V)
	}
}

func TestSplit_HexFormatHasZeroXPrefix(t *testing.T) {
	sig := make([]byte, 65)
	for i := range sig[:64] {
		sig[i] = byte(i)
	}
	out, _ := signer.Split(sig)
	if !strings.HasPrefix(out.R, "0x") || !strings.HasPrefix(out.S, "0x") {
		t.Fatalf("expected 0x-prefixed hex, got r=%s s=%s", out.R, out.S)
	}
	if len(out.R) != 66 || len(out.S) != 66 {
		t.Fatalf("expected 66-char hex, got r=%d s=%d", len(out.R), len(out.S))
	}
}

func TestSign_RoundTripWithSplit(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	digest := sha256.Sum256([]byte("hello"))
	out, err := signer.SignAndSplit(key, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if out.V != 27 && out.V != 28 {
		t.Fatalf("expected v in {27,28}, got %d", out.V)
	}
}

func TestSign_RejectsNilKey(t *testing.T) {
	if _, err := signer.Sign(nil, make([]byte, 32)); err == nil {
		t.Fatalf("expected error for nil key")
	}
}

func TestSign_RejectsNon32ByteDigest(t *testing.T) {
	key, _ := crypto.GenerateKey()
	if _, err := signer.Sign(key, make([]byte, 31)); err == nil {
		t.Fatalf("expected error for short digest")
	}
}
