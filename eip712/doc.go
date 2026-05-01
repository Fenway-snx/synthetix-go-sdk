// Package eip712 provides the typed-data construction primitives the
// Synthetix V4 trading API requires.
//
// Scope:
//
//   - Domain separator construction (Synthetix v1, mainnet).
//   - Per-action type schemas (PlaceOrders, CancelOrders, ModifyOrder,
//     UpdateLeverage, WithdrawCollateral, CreateSubaccount,
//     TransferCollateral, UpdateSubAccountName, AddDelegatedSigner,
//     RemoveDelegatedSigner, RemoveAllDelegatedSigners, ScheduleCancel,
//     AuthMessage, SubAccountAction).
//   - Per-action typed-data builders that take raw field values and
//     return an apitypes.TypedData ready for hashing or wallet display.
//   - Digest computation, signature verification, and JSON
//     serialisation for wallet round-trips.
//
// Out of scope (lives in sdk/signer):
//
//   - secp256k1 signing of a digest.
//   - Splitting a 65-byte signature into the wire-shape (r, s, v).
//   - Holding or loading private key material.
//
// The wire schema mirrors the public trading API byte-for-byte. The
// package is dependency-clean: only go-ethereum's apitypes, math, and
// crypto packages are imported.
package eip712
