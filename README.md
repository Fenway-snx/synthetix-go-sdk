# synthetix-go

Go SDK for the Synthetix V4 trading API: REST market data,
signed REST trading, public WebSockets, authenticated trade WebSockets,
and EIP-712 signing.

## Install

```bash
go get github.com/Fenway-snx/synthetix-go-sdk@latest
```

## Quickstart: Market Data

```go
ctx := context.Background()
c, err := synthetix.NewClientFromEnv()
if err != nil {
    panic(err)
}
defer c.Close()

markets, err := c.Info().GetMarkets(ctx, true)
if err != nil {
    panic(err)
}
fmt.Println(len(markets))
```

## Quickstart: Trading

Set env vars once:

```bash
export SYNTHETIX_BASE_URL=https://api.synthetix.io
export SYNTHETIX_PRIVATE_KEY=...
export SYNTHETIX_SUB_ACCOUNT_ID=1  # see "Subaccount selection" below
```

Then use the facade. It builds typed data, signs locally, and sends one
REST or WebSocket request:

```go
c, err := synthetix.NewTradingClient(ctx, synthetix.Config{
    PrivateKeyHex: os.Getenv("SYNTHETIX_PRIVATE_KEY"),
})
if err != nil {
    panic(err)
}

resp, err := c.PlaceOrder(ctx, "BTC-USDT", synthetix.SideBuy, synthetix.OrderTypeLimit, "50000", "0.01",
    synthetix.WithClientOrderID("my-order-1"),
)
market, err := c.MarketOpen(ctx, "BTC-USDT", synthetix.SideBuy, "0.01")
close, err := c.MarketClose(ctx, "BTC-USDT")
```

`ValidateAuth(ctx)` returns signer, wallet, subaccount, and private WS
readiness diagnostics when you want a pre-flight check.

Use constants and small option builders to avoid stringly typed call sites:

```go
order, err := c.PlaceOrder(ctx,
    "BTC-USDT",
    synthetix.SideBuy,
    synthetix.OrderTypeLimit,
    "50000",
    "0.01",
    synthetix.GTC(),
)
```

Price and size helpers format values to the market increments returned by
`getMarkets`:

```go
price, err := c.FormatPrice(ctx, "BTC-USDT", "50000.019")
size, err := c.FormatSize(ctx, "BTC-USDT", "0.123456")
```

## Subaccount selection

**Production systems must set `Config.SubAccountID` (or `SYNTHETIX_SUB_ACCOUNT_ID`) explicitly.**

When `SubAccountID` is zero the SDK calls `getSubAccountIds` on the first
authenticated request and silently picks `ids[0]` — whatever order the API
returns. If your wallet owns more than one subaccount (e.g. a personal
account and a fund account share the same key) all orders and cancels will go
to whichever subaccount the API happens to return first. The SDK logs a
`Warn`-level message in this case, but it is not an error.

```go
// Safe for production — routing is explicit and deterministic.
c, err := synthetix.NewTradingClient(ctx, synthetix.Config{
    PrivateKeyHex: os.Getenv("SYNTHETIX_PRIVATE_KEY"),
    SubAccountID:  1,
})
```

If you do rely on auto-discovery, call `ValidateAuth(ctx)` immediately after
construction and log the returned `SubAccountID` field so you have an audit
trail of which subaccount was selected at startup.

## Typed Filters

Common account reads have endpoint-specific filter helpers:

```go
orders, err := c.GetOrderHistory(ctx,
    synthetix.WithOrderHistoryFilter(startMs, endMs, 100),
)

fills, err := c.GetTradesForPosition(ctx,
    synthetix.WithTradesForPositionFilter(positionID, 100, 0),
)

balances, err := c.GetBalanceUpdates(ctx,
    synthetix.WithBalanceUpdatesFilter(startMs, endMs, "DEPOSIT,TRANSFER", 100, 0),
)
```

`WithParam` remains as an escape hatch for new backend fields before the
SDK grows a typed helper.

## Client order ID idempotency

`WithClientOrderID` attaches a caller-supplied string to a `PlaceOrder` call:

```go
resp, err := c.PlaceOrder(ctx, "BTC-USDT", synthetix.SideBuy, synthetix.OrderTypeLimit,
    "50000", "0.01",
    synthetix.WithClientOrderID("my-order-1"),
)
```

**Behavioral contract (as of v0.1):**

- The client order ID is passed verbatim to the exchange in the signed EIP-712
  envelope; the SDK does **not** enforce uniqueness or deduplication itself.
- The exchange deduplicates on `(walletAddress, clientOrderID)` per subaccount
  for a venue-defined window. A second `PlaceOrder` carrying the same client
  order ID within that window is treated as an idempotent retry and returns the
  original order rather than creating a duplicate — **but this is exchange
  policy, not SDK behaviour**, and should not be relied on across reconnects or
  after the deduplication window expires.
- Cancel-by-client-order-ID (`CancelByClientOrderID`) is a distinct RPC; a
  pending or filled order can always be located by its client order ID via
  `GetOpenOrders` / `GetOrderHistory`.
- If you need strict at-most-once semantics, check `GetOpenOrders` for the
  client order ID before retrying a failed call.

## WebSockets

`WSInfo()` is for public streams. `WSTrade()` is for authenticated private
streams and signed trade posts. Go uses callbacks or channels instead of
Python-style `async` methods.

```go
unsubscribe, err := c.SubscribeOrderbook(ctx, "BTC-USDT", func(msg *types.WSMessage) {
    fmt.Println(string(msg.Raw))
}, synthetix.WithDepth(10))
defer unsubscribe()
```

For private account updates, use the channel helper:

```go
updates, stop, err := c.WSTrade().SubscribeSubAccountUpdatesChan(ctx, 1, 32)
defer stop()

for msg := range updates {
    fmt.Println(string(msg.Raw))
}
```

## Dead-man switch (`ScheduleCancel`)

`ScheduleCancel` arms or disarms the exchange's dead-man switch for a
subaccount.

```go
// Arm: cancel all open orders if no heartbeat arrives within 60 s.
resp, err := c.ScheduleCancel(ctx, 60)

// Disarm: pass 0 to clear the schedule.
resp, err := c.ScheduleCancel(ctx, 0)
```

**Semantics to be aware of:**

- The timer is maintained **server-side**. Once armed it ticks independently
  of the SDK process; a clean process exit does **not** disarm it.
- On WebSocket reconnect the SDK re-authenticates the trade socket but does
  **not** automatically re-arm `ScheduleCancel`. If your architecture relies
  on the switch being armed at all times, call `ScheduleCancel` again
  immediately after a successful reconnect or use the REST transport
  (`OverWS()` not set) and poll from a dedicated heartbeat goroutine.
- Disarming is the same call with `timeoutSeconds == 0`. There is no separate
  `CancelScheduleCancel` method.
- The REST and WebSocket transports both work; REST is simpler to reason about
  for a heartbeat loop because it is stateless.

## WebSocket reliability

Both `WSInfo` (public streams) and `WSTrade` (authenticated streams) reconnect
automatically on disconnect and re-subscribe / re-authenticate, but there are
gaps you must account for in production:

**Message delivery is not guaranteed across reconnects.** The SDK does not
track sequence numbers or detect missing frames. If the connection drops and
reconnects between two fills, those fills are silently absent from the callback
stream. Treat WebSocket data as a low-latency view; always reconcile positions
and fills against the REST endpoints (`GetPositions`, `GetTradesForPosition`)
after any reconnect.

**Slow consumers lose messages.** The `wsinfo` fan-out uses a per-subscriber
ring buffer. If your callback cannot keep up with the inbound rate, oldest
messages are dropped rather than blocking the read loop. This is intentional;
a fast handler is your responsibility. The `wstrade` channel helpers
(`SubscribeSubAccountUpdatesChan`) let you specify a buffer size — size it for
your expected burst.

**Orderbook sequence validation is not implemented in v1.** The wire format
includes `seq` / `prev_seq` fields on `types.WSMessage` but the SDK does not
assert continuity; callers that need gap detection must check `PrevSeq`
themselves.

**Heartbeats run automatically.** Both clients send an application-level
`{"method":"ping"}` on a configurable interval (default `DefaultPingInterval`)
and refresh the read deadline on every inbound frame. A stalled connection that
misses the read deadline triggers an automatic reconnect and re-subscription.

## Lower-Level Packages

- `synthetix`: top-level facade with one-line helpers.
- `restinfo`: typed `/v1/info` transport.
- `resttrade`: typed `/v1/trade` transport for already-signed envelopes.
- `wsinfo`: public `/v1/ws/info` subscriptions.
- `wstrade`: authenticated `/v1/ws/trade` posts and account updates.
- `signer`: private-key EIP-712 signing helpers.
- `eip712`: typed-data builders.
- `types`: shared wire structs.

## Examples

The `examples/` directory contains copy-pasteable programs for market data,
account reads, order lifecycle actions, delegation, and WebSockets. Start with
`examples/basic_order` and `examples/basic_ws`. Configure them with:

```bash
export SYNTHETIX_BASE_URL=https://api.synthetix.io
export SYNTHETIX_PRIVATE_KEY=...
export SYNTHETIX_SUB_ACCOUNT_ID=1
go run ./examples/order_limit
```

## Development

```bash
make test
make vet
make examples-build
make lint
```

## Authentication

`/v1/info` is unauthenticated. Everything on `/v1/trade` and the
authenticated WebSocket streams require a fresh EIP-712 signature on
each request.

The standard high-level flow:

1. Construct a trading client with `NewClientFromEnv`, `NewTradingClient`,
   or `NewClient(Config{PrivateKeyHex: ...})`.
2. Optionally call `ValidateAuth(ctx)` to confirm signer, wallet,
   subaccount, and trade websocket readiness.
3. Call high-level helpers such as `PlaceOrder`, `GetPositions`, or
   `ScheduleCancel`.

The low-level flow remains available for KMS, hardware wallet, or service
signer integrations:

1. Build the typed-data payload via `eip712.BuildAuthMessage` (or one
   of the trade-action builders for `/v1/trade`).
2. Sign + split with `signer.SignTypedDataAndSplit(privKey, typedData)`
   — returns the wire-shape `{r, s, v}` triple every Synthetix
   verifier expects.
3. Hand the signature to the relevant `resttrade` or `wstrade` method.

If you'd rather not handle keys directly, the `synthetix` facade
accepts a `PrivateKeyHex` in `Config` and exposes `Client.Signer()`
for the same primitives.

## Error types

The SDK exposes a small typed error hierarchy so callers can inspect failures
without string matching.

| Type | Package | When returned |
|---|---|---|
| `ErrNoSigner` | `synthetix` | Trading call made without a private key |
| `TransportError` | `restinfo`, `resttrade` | Non-2xx HTTP with no parseable body |
| `RESTError` | `restinfo`, `resttrade` | Exchange returned `{"error": ...}` JSON envelope |
| `ErrMarketNotFound` | `restinfo` | Symbol not in `getMarkets` response |
| `WSReplyError` | `wsinfo` | Exchange returned an error frame on a subscribe/unsubscribe/ping |
| `WSError` | `wstrade` | Exchange returned an error frame on auth or a trade post |

`RESTError` carries `Code` (string) and `Message` (string) exactly as the
exchange sends them. Venue-level rejections (insufficient margin, rate limit,
duplicate order, etc.) arrive through `RESTError.Code`; the SDK does not yet
map these to typed sentinel values. Use `errors.As` to extract the code and
compare against exchange documentation:

```go
resp, err := c.PlaceOrder(ctx, ...)
var re *restinfo.RESTError
if errors.As(err, &re) {
    switch re.Code {
    case "INSUFFICIENT_MARGIN":
        // handle
    case "RATE_LIMIT_EXCEEDED":
        // back off
    }
}
```

Order-level rejections (where HTTP 200 is returned but the order itself
failed) are available as `types.OrderStatus.Error` and `.ErrorCode` fields on
the response struct.

## Logging

Pass any implementation of `logger.Logger`. `slog`, `zerolog`, `zap`
and the stdlib `log` package can each be wrapped in a few lines. Pass
`nil` (or `logger.Nop()`) to drop SDK output entirely. The SDK emits
**no** stdout/stderr by default — consumers stay in control.

## Versioning + stability

- This SDK is pre-1.0. Minor versions may break public API; every
  break ships in release notes.
- Once a stable surface is reviewed by external consumers, this repo
  will tag `v1.0.0` and follow [semver](https://semver.org/) strictly.
- The wire format on `api.synthetix.io` is a separate contract owned
  by the Synthetix backend; SDK releases track upstream additions but
  never silently change wire shapes.

## Contributing

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for the dev loop, commit
conventions, and how to run tests locally.

## Security

Security disclosures: see [`SECURITY.md`](./SECURITY.md). Please
**do not** file public issues for vulnerabilities.

## License

[MIT](./LICENSE) © Synthetix.
