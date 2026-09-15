package cloudflare

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/go-cdnkit/nozzle"
)

// Status describes what is known about an operation's submission.
type Status uint8

const (
	// NotAttempted means the operation was not handed to the HTTP client.
	NotAttempted Status = iota
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

// Execute returns no results for an empty plan.
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
	if len(results) > 0 {
		if err := ctx.Err(); err != nil {
			return results, err
		}
	}
	return results, nil
}
