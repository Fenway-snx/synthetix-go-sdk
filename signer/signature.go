package signer

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// On-wire EIP-712 signature triple. JSON tags match the public REST
// /v1/trade body shape and the TS SDK.
type Signature struct {
	R string `json:"r"`
	S string `json:"s"`
	V uint8  `json:"v"`
}

// Splits a raw 65-byte secp256k1 signature into the wire-shape
// triple. Normalises v to the legacy 27/28 encoding accepted by the
// Synthetix API. Returns an error if the input is not exactly 65
// bytes.
func Split(signatureBytes []byte) (Signature, error) {
	if len(signatureBytes) != 65 {
		return Signature{}, fmt.Errorf("signer: signature must be 65 bytes, got %d", len(signatureBytes))
	}
	v := signatureBytes[64]
	if v < 27 {
		v += 27
	}
	return Signature{
		R: "0x" + common.Bytes2Hex(signatureBytes[0:32]),
		S: "0x" + common.Bytes2Hex(signatureBytes[32:64]),
		V: v,
	}, nil
}
