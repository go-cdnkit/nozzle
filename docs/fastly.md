# Fastly

Fastly currently supports configuration validation and offline URL planning only.
It does not send purge requests or invalidate caches.

## Configuration

`fastly.New(fastly.Config{APIToken: token, HTTPClient: client})` requires a
non-empty token without whitespace or control characters and a non-nil HTTP
client. Configuration errors never include the token.

The provider stores a copy of the configuration. The HTTP client remains
caller-owned; it is not cloned, modified, or used during construction or planning.
No environment variables or default clients are used. Credential validity and
Fastly service eligibility are not checked.

## Planning

`provider.Plan(urls)` returns a `*nozzle.Plan` with one exact URL per operation.
It preserves URL text, input order, and duplicates without network requests.
Empty input returns an empty, non-nil plan. Invalid input returns no plan and an
`*nozzle.InvalidTargetError` containing the original zero-based input index.

Each operation owns its URL slice independently of the input and other plans.
Plans remain editable and are not bound to a provider. A successful plan does
not establish Fastly routing, cache-key coverage, or purge acceptance.
