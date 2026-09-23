package cloudflare

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
type OperationError struct {
	Index int
	Err   error
}

func (e *OperationError) Error() string {
	return fmt.Sprintf("cloudflare: operation %d: %v", e.Index, e.Err)
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
		return nil, errors.New("cloudflare: nil execution plan")
	}
	results := make([]nozzle.OperationResult, len(plan.Operations))
	for i, operation := range plan.Operations {
		results[i].Operation.URLs = slices.Clone(operation.URLs)
	}
	for i, result := range results {
		urls := result.Operation.URLs
		if len(urls) == 0 {
			return results, &OperationError{Index: i, Err: errors.New("empty operation")}
		}
		if len(urls) > p.config.MaxURLsPerRequest {
			return results, &OperationError{Index: i, Err: errors.New("operation exceeds the configured URL limit")}
		}
		if _, err := nozzle.PlanURLs(urls, p.config.MaxURLsPerRequest); err != nil {
			return results, &OperationError{Index: i, Err: err}
		}
	}
	for i := range results {
		if err := ctx.Err(); err != nil {
			return results, &OperationError{Index: i, Err: err}
		}
		status, httpStatus, err := p.executeOperation(ctx, results[i].Operation)
		results[i].Status = status
		results[i].HTTPStatus = httpStatus
		if err != nil {
			return results, &OperationError{Index: i, Err: err}
		}
	}
	return results, nil
}

func (p *Provider) executeOperation(ctx context.Context, operation nozzle.Operation) (nozzle.Status, int, error) {
	req, err := newBatchPurgeRequest(ctx, p.config.ZoneID, p.config.APIToken, operation.URLs)
	if err != nil {
		return nozzle.NotAttempted, 0, err
	}
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
	success, err := parsePurgeResponse(body)
	if err != nil {
		return nozzle.Indeterminate, resp.StatusCode, err
	}
	if !success && (resp.StatusCode >= 200 && resp.StatusCode < 300 || resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusRequestTimeout) {
		return nozzle.Rejected, resp.StatusCode, errors.New("purge operation was rejected")
	}
	if !success || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nozzle.Indeterminate, resp.StatusCode, errors.New("purge acceptance was not confirmed")
	}
	return nozzle.Accepted, resp.StatusCode, nil
}
