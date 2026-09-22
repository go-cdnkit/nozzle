# Fastly

Fastly supports offline exact-URL planning and sequential purge execution.

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

## Execution

`provider.Execute(ctx, plan)` returns `[]fastly.OperationResult` and an error.
It snapshots and validates every operation before sending anything. Each operation
must contain exactly one URL; multi-URL operations are rejected, not regrouped.
A nil plan fails. An empty plan succeeds without network I/O.

Requests use `POST https://api.fastly.com/purge/{url}`, `Fastly-Key`, and no body.
URL text, query order, repeated query keys, and duplicate targets are preserved;
ordinary Go HTTP serialization still applies to raw non-ASCII text. Requests never
go directly to the target origin. Redirects are not followed.

Operations run in order and stop at the first error. Results include every
operation, including earlier accepted work and later unattempted work. Result URL
slices are independent copies; do not mutate the input while it is being copied.

| Status | Meaning |
| --- | --- |
| `NotAttempted` | Not handed to the HTTP client. |
| `Accepted` | A complete, unambiguous API response confirmed acceptance. |
| `Rejected` | A recognized API error response explicitly refused the operation. |
| `Indeterminate` | Submission began but acceptance could not be established. |

Acceptance requires a 2xx response with `status: "ok"` and no non-empty
`msg`, `detail`, or `errors`. Rejection requires a 4xx response other than 408,
no `status`, a non-empty legacy `msg`, and no non-empty `errors` array.
Malformed, contradictory, unsupported, or oversized responses are indeterminate.
Response reads are limited to 64 KiB plus one overflow-detection byte.

`HTTPStatus` is zero when no response status was obtained. `*fastly.OperationError`
identifies the zero-based failing operation. Use `errors.Is` and `errors.As` to
inspect cancellation or transport causes. The outer error string omits targets,
tokens, and remote response text; explicitly unwrapped errors and result targets
can still contain sensitive information. Avoid logging them indiscriminately.

Supply a non-nil context and a client with suitable deadlines. Execution uses a
shallow client copy to disable redirects, preserving the caller's transport,
timeout, and cookie jar. No caller client settings are changed. No retry, sleep,
or background worker is introduced; a custom transport may perform its own retries.

## Limits

`Accepted` is API acceptance, not proof of global cache invalidation. Planning
does not infer custom cache keys, rewriting, service routing, or `Vary` coverage.
Soft purge, surrogate keys, full-cache purge, and completion polling are unsupported.

The fixed HTTPS API endpoint constrains nozzle's request destination, not Fastly's
downstream behavior. Review Fastly's [purge authentication guidance](https://www.fastly.com/documentation/guides/full-site-delivery/purging/authenticating-api-purge-requests/),
including its warning about tokens and non-HTTPS sites, before live use. Tests use
local TLS servers; they do not establish live credential safety or cache coverage.
