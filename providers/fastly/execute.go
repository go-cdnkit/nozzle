package fastly

import (
	"context"
	"errors"
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

// Execute validates a snapshot of the whole plan before any network I/O.
func (p *Provider) Execute(ctx context.Context, plan *nozzle.Plan) ([]OperationResult, error) {
	if plan == nil {
		return nil, errors.New("fastly: nil execution plan")
	}
	results := make([]OperationResult, len(plan.Operations))
	for i, operation := range plan.Operations {
		results[i].Operation.URLs = slices.Clone(operation.URLs)
	}
	for _, result := range results {
		if len(result.Operation.URLs) != 1 {
			return results, errors.New("fastly: operation does not contain exactly one URL")
		}
		if _, err := nozzle.PlanURLs(result.Operation.URLs, 1); err != nil {
			return results, err
		}
	}
	if len(results) != 0 {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		return results, errors.New("fastly: non-empty execution is not implemented yet")
	}
	return results, nil
}
