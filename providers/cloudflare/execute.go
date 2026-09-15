package cloudflare

import (
	"context"
	"encoding/json"
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
// A nested nozzle.InvalidTargetError identifies the URL within that operation.
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

// Execute validates a snapshot of the whole plan before submitting operations in order.
// It returns all operation results, including unattempted ones, on the first error.
func (p *Provider) Execute(ctx context.Context, plan *nozzle.Plan) ([]OperationResult, error) {
	if plan == nil {
		return nil, errors.New("cloudflare: nil execution plan")
	}
	results := make([]OperationResult, len(plan.Operations))
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

func (p *Provider) executeOperation(ctx context.Context, operation nozzle.Operation) (Status, int, error) {
	req, err := newBatchPurgeRequest(ctx, p.config.ZoneID, p.config.APIToken, operation.URLs)
	if err != nil {
		return NotAttempted, 0, err
	}
	resp, err := p.config.HTTPClient.Do(req)
	if err != nil {
		return Indeterminate, 0, err
	}
	defer func() {
		// The complete response determines the outcome; Close releases resources.
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Indeterminate, resp.StatusCode, err
	}
	var response struct {
		Success *bool `json:"success"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return Indeterminate, resp.StatusCode, errors.New("invalid purge response")
	}
	if response.Success != nil && !*response.Success && (resp.StatusCode >= 200 && resp.StatusCode < 300 || resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusRequestTimeout) {
		return Rejected, resp.StatusCode, errors.New("purge operation was rejected")
	}
	if response.Success == nil || !*response.Success || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Indeterminate, resp.StatusCode, errors.New("purge acceptance was not confirmed")
	}
	return Accepted, resp.StatusCode, nil
}
