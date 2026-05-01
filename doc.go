// Package sdk is the top-level marker for the Synthetix V4 Go SDK. It
// contains no code itself; importable functionality lives
// in the subpackages:
//
//   - synthetix: top-level convenience facade
//   - restinfo:  /v1/info typed client (read-only)
//   - resttrade: /v1/trade typed client (authenticated)
//   - wsinfo:    streaming WebSocket client
//   - types:     shared wire-shape structs
//   - signer:    pure-crypto signing helpers
//   - logger:    BYO-logger interface
//
// See the README in this directory for the public API tour and
// integration patterns.
package sdk
