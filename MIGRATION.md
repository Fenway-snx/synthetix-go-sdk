# Migration Notes

This release expands the SDK from low-level transports into a full
facade-style trading SDK.

## WebSocket Accessors

Use `Client.WSInfo()` for public market-data streams and `Client.WSTrade()`
for authenticated trade streams. `Client.WS()` remains as a deprecated alias
for `WSInfo()` during the v0.x line.

## Signer Return Types

Several signer helpers now return dedicated request types instead of the
generic `*types.SignedEnvelope`:

- `SignUpdateLeverage`
- `SignWithdrawCollateral`
- `SignTransferCollateral`
- `SignScheduleCancel`
- `SignRemoveDelegatedSigner`

Pass those directly to the corresponding `resttrade.Client` methods.

## High-Level Helpers

Authenticated helpers are available on `*synthetix.Client`, for example
`PlaceOrder`, `MarketOrder`, `CancelOrder`, `GetPositions`,
`UpdateLeverage`, and `AddDelegatedSigner`. These helpers require
`Config.PrivateKeyHex`; set `Config.SubAccountID` or allow automatic
subaccount discovery.

## Auth UX Helpers

New constructors and diagnostics make the facade the default auth path:

- `ConfigFromEnv` and `NewClientFromEnv` read `SYNTHETIX_BASE_URL`,
  `SYNTHETIX_PRIVATE_KEY`, `SYNTHETIX_SUB_ACCOUNT_ID`, and
  `SYNTHETIX_EXPIRES_AFTER_MS`.
- `NewReadOnlyClient` and `NewTradingClient` make auth mode explicit.
- `DiscoverDefaultSubAccount`, `DefaultSubAccountID`, and
  `SetDefaultSubAccountID` expose subaccount discovery/control.
- `AuthStatus` and `ValidateAuth` report signer, wallet, subaccount, and
  trade websocket readiness.

