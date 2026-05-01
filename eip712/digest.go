package eip712

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

var errSignatureMustBe65Bytes = errors.New("eip712: signature must be 65 bytes")

// TypedData is the canonical EIP-712 typed-data shape returned by
// every Build* function in this package. Aliased so SDK callers do
// not need an explicit dependency on go-ethereum's signer/core/apitypes.
type TypedData = apitypes.TypedData

// Digest computes the EIP-712 message hash for the given typed data.
// The result is keccak256("\x19\x01" ‖ domainSeparator ‖
// hashStruct(message)) per the spec, suitable for direct passing to
// secp256k1 sign / verify.
func Digest(typedData apitypes.TypedData) ([]byte, error) {
	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		return nil, fmt.Errorf("eip712: hash domain: %w", err)
	}
	messageHash, err := typedData.HashStruct(typedData.PrimaryType, typedData.Message)
	if err != nil {
		return nil, fmt.Errorf("eip712: hash message: %w", err)
	}
	rawData := []byte(fmt.Sprintf("\x19\x01%s%s", string(domainSeparator), string(messageHash)))
	return crypto.Keccak256(rawData), nil
}

// RecoverSigner recovers the Ethereum address that produced an
// EIP-712 signature over the given typed data. The signature's
// recovery id (v) is normalised from the Ethereum convention
// (27 / 28) to the crypto library convention (0 / 1) before recovery.
//
// signatureHex may be supplied with or without the 0x prefix.
func RecoverSigner(typedData apitypes.TypedData, signatureHex string) (common.Address, error) {
	if len(signatureHex) >= 2 && (signatureHex[:2] == "0x" || signatureHex[:2] == "0X") {
		signatureHex = signatureHex[2:]
	}

	signature := common.FromHex("0x" + signatureHex)
	if len(signature) != 65 {
		return common.Address{}, errSignatureMustBe65Bytes
	}

	digest, err := Digest(typedData)
	if err != nil {
		return common.Address{}, err
	}

	if signature[64] >= 27 {
		signature[64] -= 27
	}

	pubKey, err := crypto.SigToPub(digest, signature)
	if err != nil {
		return common.Address{}, fmt.Errorf("eip712: recover public key: %w", err)
	}
	return crypto.PubkeyToAddress(*pubKey), nil
}

// Serialize converts typed data to the JSON shape browser wallets
// expect on eth_signTypedData_v4. Suitable for handing off to a
// MetaMask-style external signer when the SDK is used in a
// wallet-driven context.
func Serialize(typedData apitypes.TypedData) (string, error) {
	data := map[string]any{
		"types":       typedData.Types,
		"primaryType": typedData.PrimaryType,
		"domain":      typedData.Domain.Map(),
		"message":     typedData.Message,
	}
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("eip712: serialize typed data: %w", err)
	}
	return string(jsonBytes), nil
}
