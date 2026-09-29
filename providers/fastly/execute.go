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

// OperationError identifies the zero-based operation index and preserves its cause.
// Its text omits the cause, which remains inspectable and may contain secrets.
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

// Execute validates a copy of the entire plan, then submits operations in order.
// It stops on the first error, retaining every result, including unattempted work.
// It neither follows redirects nor schedules retries.
// A nil plan fails; an empty plan succeeds without I/O.
// Provide a non-nil context and do not mutate the plan while it is being copied.
func (p *Provider) Execute(ctx context.Context, plan *nozzle.Plan) ([]nozzle.OperationResult, error) {
	if plan == nil {
		return nil, errors.New("fastly: nil execution plan")
	}
	results := make([]nozzle.OperationResult, len(plan.Operations))
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

func (p *Provider) executeOperation(ctx context.Context, target string) (nozzle.Status, int, error) {
	// Keep the destination fixed without normalizing the embedded purge target.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.fastly.com/purge/"+target, nil)
	if err != nil {
		return nozzle.NotAttempted, 0, err
	}
	req.Header.Set("Fastly-Key", p.config.APIToken)
	req.Header.Set("Accept", "application/json")
	client := *p.config.HTTPClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Do(req)
	if err != nil {
		return nozzle.Indeterminate, 0, err
	}
	defer func() {
		// A close error does not change the response outcome.
		_ = resp.Body.Close()
	}()
	const maxResponseBytes = 64 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nozzle.Indeterminate, resp.StatusCode, err
	}
	if len(body) > maxResponseBytes {
		return nozzle.Indeterminate, resp.StatusCode, errors.New("purge response exceeds 64 KiB")
	}
	status, err := parsePurgeResponse(body, resp.StatusCode)
	return status, resp.StatusCode, err
}
