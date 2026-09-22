package fastly

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"

	"github.com/go-cdnkit/nozzle"
)

// Status describes what is known about an operation's submission.
type Status uint8

const (
	// NotAttempted means the operation was not handed to the HTTP client.
	NotAttempted Status = iota
	// Accepted means the API confirmed acceptance, not global cache invalidation.
	Accepted
	// Rejected means the API explicitly refused the operation.
	Rejected
	// Indeterminate means submission began without a usable confirmation.
	Indeterminate
)

// OperationResult associates an execution outcome with independently owned targets.
type OperationResult struct {
	Operation  nozzle.Operation
	Status     Status
	HTTPStatus int
}

// OperationError identifies the zero-based operation index and preserves its cause.
// Its text omits the cause, which may contain URLs or credentials. Inspect Err or
// use errors.Is/errors.As explicitly when those diagnostics are needed.
type OperationError struct {
	Index int
	Err   error
}

func (e *OperationError) Error() string {
	return fmt.Sprintf("fastly: operation %d failed", e.Index)
}

func (e *OperationError) Unwrap() error {
	return e.Err
}

// Execute validates a snapshot of the whole plan before submitting operations in order.
// It returns every operation's result, including unattempted ones, on the first error.
// A nil plan fails; an empty plan succeeds without network I/O. The caller must
// supply a non-nil context and must not mutate the plan during snapshotting.
// Accepted confirms API acceptance only, not completion of cache invalidation.
func (p *Provider) Execute(ctx context.Context, plan *nozzle.Plan) ([]OperationResult, error) {
	if plan == nil {
		return nil, errors.New("fastly: nil execution plan")
	}
	results := make([]OperationResult, len(plan.Operations))
	for i, operation := range plan.Operations {
		results[i].Operation.URLs = slices.Clone(operation.URLs)
	}
	for i, result := range results {
		if len(result.Operation.URLs) != 1 {
			return results, &OperationError{Index: i, Err: errors.New("operation does not contain exactly one URL")}
		}
		if _, err := nozzle.PlanURLs(result.Operation.URLs, 1); err != nil {
			return results, &OperationError{Index: i, Err: err}
		}
	}
	for i := range results {
		if err := ctx.Err(); err != nil {
			return results, &OperationError{Index: i, Err: err}
		}
		status, httpStatus, err := p.executeOperation(ctx, results[i].Operation.URLs[0])
		results[i].Status = status
		results[i].HTTPStatus = httpStatus
		if err != nil {
			return results, &OperationError{Index: i, Err: err}
		}
	}
	return results, nil
}

func (p *Provider) executeOperation(ctx context.Context, target string) (Status, int, error) {
	// Keep the destination fixed without normalizing the embedded purge target.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.fastly.com/purge/"+target, nil)
	if err != nil {
		return NotAttempted, 0, err
	}
	req.Header.Set("Fastly-Key", p.config.APIToken)
	req.Header.Set("Accept", "application/json")
	client := *p.config.HTTPClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Do(req)
	if err != nil {
		return Indeterminate, 0, err
	}
	defer func() {
		// The complete response determines the outcome; Close releases resources.
		_ = resp.Body.Close()
	}()
	const maxResponseBytes = 64 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return Indeterminate, resp.StatusCode, err
	}
	if len(body) > maxResponseBytes {
		return Indeterminate, resp.StatusCode, errors.New("purge response exceeds 64 KiB")
	}
	status, err := parsePurgeResponse(body, resp.StatusCode)
	return status, resp.StatusCode, err
}
