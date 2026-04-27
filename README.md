# synthetix-go

Go SDK for the Synthetix V4 off-chain trading API: REST market data,
signed REST trading, public WebSockets, authenticated trade WebSockets,
and EIP-712 signing.

## Install

```bash
go get github.com/synthetixio/synthetix-go@latest
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
export SYNTHETIX_SUB_ACCOUNT_ID=1 # optional; discovered when omitted
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
