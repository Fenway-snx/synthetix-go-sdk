package eip712

import (
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

const (
	// DefaultDomainName is the EIP-712 domain name the API expects on
	// every signed request.
	DefaultDomainName = "Synthetix"

	// DefaultDomainVersion is the EIP-712 domain version.
	DefaultDomainVersion = "1"

	// DefaultChainID is the chain id used in the domain. The
	// Synthetix off-chain stack pins this to mainnet (1) regardless
	// of the actual deployment environment because the EIP-712
	// signature is verified off-chain by the API.
	DefaultChainID = 1

	// ZeroAddress is the verifyingContract value used in the
	// domain separator. The off-chain stack does not have a
	// verifying contract on-chain.
	ZeroAddress = "0x0000000000000000000000000000000000000000"
)

// DomainFields returns the canonical EIP712Domain field list. Used by
// every typed-data builder.
func DomainFields() []apitypes.Type {
	return []apitypes.Type{
		{Name: "name", Type: "string"},
		{Name: "version", Type: "string"},
		{Name: "chainId", Type: "uint256"},
		{Name: "verifyingContract", Type: "address"},
	}
}

// Domain returns the apitypes.TypedDataDomain for a given name,
// version, and chain id. Pass DefaultDomainName, DefaultDomainVersion,
// DefaultChainID for the standard Synthetix domain.
func Domain(name, version string, chainID int) apitypes.TypedDataDomain {
	return apitypes.TypedDataDomain{
		Name:              name,
		Version:           version,
		ChainId:           math.NewHexOrDecimal256(int64(chainID)),
		VerifyingContract: ZeroAddress,
	}
}

// DefaultDomain returns the standard Synthetix domain separator.
// Equivalent to Domain(DefaultDomainName, DefaultDomainVersion,
// DefaultChainID).
func DefaultDomain() apitypes.TypedDataDomain {
	return Domain(DefaultDomainName, DefaultDomainVersion, DefaultChainID)
}
