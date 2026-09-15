# Contributing

Use Go 1.26.0 or newer and golangci-lint v2.13.1.
See the [official linter installation guide](https://golangci-lint.run/docs/welcome/install/).

Run `make check` before submitting a change. The individual commands are:

| Target | Command | Purpose |
| --- | --- | --- |
| `make fmt` | `gofmt -w .` | Format Go files in place. |
| `make fmt-check` | `gofmt -l .` with an empty-output check | Reject unformatted files without changing them. |
| `make deps-check` | `go mod tidy -diff` | Reject module drift without changing files. |
| `make test` | `go test ./...` | Run package tests. |
| `make test-race` | `go test -race ./...` | Run tests with the race detector; requires a supported platform and C compiler. |
| `make lint` | `golangci-lint run ./...` | Run the standard linters and gofmt checks. |

The package supports offline URL planning and sequential Cloudflare execution.
Local-server tests verify request construction and result handling, not live
provider acceptance or global cache invalidation. Add focused `_test.go` tests
alongside each implementation, using a local HTTP server for provider behavior
rather than live CDN credentials.

Keep changes focused. For a new provider, open an issue with its API documentation
first. Never commit tokens or other credentials.

## CI

- Package tests run on Linux amd64, macOS arm64, and Windows amd64 with Go 1.26.0
  and stable Go. Linux arm64 runs stable Go. These are native test runs, not just
  cross-compilation checks.
- Package tests disable CGO. A separate Linux amd64 job enables CGO and runs
  `go test -race ./...` with stable Go.
- Formatting, module drift, and lint checks run on Linux with the Go version in
  `go.mod`. Platform test jobs use `go test ./...` directly, without requiring Make.

CI runs for pull requests and pushes to `main`, and can also be started manually.
Tests must not require live CDN credentials or mutate real CDN caches.
