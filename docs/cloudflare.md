# Cloudflare

Unreleased API for exact-URL invalidation. Planning is offline; execution sends
requests to the Cloudflare purge API. No account settings are discovered.

## API

Construct a provider with `cloudflare.New(cloudflare.Config{...})`, supplying a
zone ID, API token, HTTP client and `MaxURLsPerRequest`. Choose a capacity supported
by your account within the library's accepted range of 1–500 URLs per operation.
Set an appropriate client timeout or context deadline; nozzle supplies neither.

`provider.Plan(urls)` returns an editable `*nozzle.Plan`. It preserves URL text,
order and duplicates, and groups targets without network I/O. Your application
decides which public URLs changed; nozzle does not discover them.

`provider.Execute(ctx, plan)` returns `[]cloudflare.OperationResult` and an error.
It copies all targets, validates the entire plan, then submits operations in order.
Invalid input sends nothing. A nil plan fails; an empty plan is a successful no-op.
Supply a non-nil context and do not mutate the plan during its initial snapshot.
Do not mutate the HTTP client or transport configuration during execution either.

Execution stops at the first error. Inspect results even when the error is non-nil:
there is one result per supplied operation, including those not attempted, with
independently owned URL slices. No regrouping or full-cache fallback occurs.

## Results

| Status | Meaning |
| --- | --- |
| `NotAttempted` | The operation was not handed to the HTTP client. |
| `Accepted` | A complete 2xx JSON response explicitly confirms acceptance without conflicting errors. |
| `Rejected` | A complete JSON response explicitly refuses the operation on 2xx or 4xx other than 408. |
| `Indeterminate` | Submission began, but there is no trustworthy acceptance or rejection confirmation. |

`Accepted` does not prove global cache invalidation. Transport failures, interrupted
or malformed responses, redirects, 408 and 5xx are indeterminate. Even a dial error
is conservatively indeterminate once the HTTP client has received the request.
A 429 is rejected only when its complete response explicitly says `success=false`.

Each result retains the received HTTP status, or zero when no status was received.
An `*cloudflare.OperationError` identifies the zero-based operation index and unwraps
its cause for `errors.Is` and `errors.As`. A nested `*nozzle.InvalidTargetError`
identifies the URL's index within that operation. A nil-plan error has no operation.

## Boundaries

- Redirects are disabled on a request-local client copy; the caller's client stays
  unchanged. Its transport, timeout and cookie jar are shared, not replaced.
- Complete response bodies are limited to 64 KiB. Missing, duplicated or contradictory
  confirmation fields are not accepted. Unknown non-critical metadata is tolerated.
- No automatic retries, sleeps or background workers are added. A caller-supplied
  transport may retry internally, so one operation need not mean one network attempt.
- Generated errors omit raw response text, credentials and target URLs. Preserved
  transport errors may contain caller-owned diagnostics. Results deliberately retain
  target URLs; avoid logging sensitive query strings.
- Planning does not verify credentials, zone membership, account limits or custom
  cache-key coverage. Only URL-string purges are supported, not cache-key headers,
  tags, prefixes, wildcards or full-cache purges.
- Applications decide how to respond to partial or uncertain results. Do not blindly
  replay an entire plan when earlier operations were already accepted.
