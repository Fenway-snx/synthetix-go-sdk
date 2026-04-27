// Package signer signs Synthetix EIP-712 typed data and returns the
// signed request envelopes expected by the public trading API.
//
// Low-level helpers are also available for callers that construct
// typed data themselves: Sign produces a raw secp256k1 signature and
// Split converts it into the {r, s, v} triple used on the wire.
package signer
