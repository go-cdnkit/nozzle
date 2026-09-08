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

The package currently contains documentation only. `[no test files]` confirms
compilation, not working CDN behavior. Add focused `_test.go` tests alongside
each implementation, using a local HTTP server rather than live CDN credentials.

Keep changes focused. For a new provider, open an issue with its API documentation
first. Never commit tokens or other credentials.
