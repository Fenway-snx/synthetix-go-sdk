# Contributing to synthetix-go

Thanks for picking up a PR. The SDK is small on purpose; keep it that
way.

## Dev loop

```bash
git clone https://github.com/Fenway-snx/synthetix-go-sdk.git
cd synthetix-go
go mod download
go test ./...
```

The test suite is hermetic — every REST and WebSocket interaction is
served by an in-process `httptest.Server`. No network, no creds, no
fixtures to refresh. If you're adding a new endpoint, mirror the
existing `*_test.go` patterns rather than introducing a live test.

## Before you open a PR

- `go build ./...`
- `go vet ./...`
- `go test -race ./...`
- `gofmt -s -w .`

If your change adds a public API, add at least one happy-path test
that exercises it through the relevant `httptest.Server` fake.

## Commit style

- Imperative subject line, ≤72 chars (`signer: split v normalisation
  into helper`).
- Body explains *why* — what behaviour changes for callers, what
  trade-offs you considered. The diff already explains *what*.
- Prefix the subject with the package you're touching when it scopes
  cleanly (`signer:`, `wsinfo:`, `eip712:`).
- One logical change per commit. Mechanical churn (gofmt, import
  reorders) goes in its own commit.

## Public API discipline

This SDK is pre-1.0, but consumers already depend on its exported
surface. Treat every exported identifier as a contract:

- Renames need an alias for at least one minor release.
- New required fields on `Config` structs are a breaking change;
  prefer defaulting in the constructor.
- Changes to wire-shape structs in `types/` must match a real
  `api.synthetix.io` change. If the upstream contract hasn't moved,
  neither should the type.

## Logging

Don't import a concrete logger. Use the `logger.Logger` interface and
let the consumer pick (`slog`, `zerolog`, `zap`, `nil`). The SDK is
expected to be silent by default.

## Reporting bugs

Open an issue with a reproducer (Go program ≤30 lines preferred). If
the bug is security-sensitive, follow [`SECURITY.md`](./SECURITY.md)
instead.
