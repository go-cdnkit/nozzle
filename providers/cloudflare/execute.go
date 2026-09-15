package cloudflare

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
)

// OperationResult associates an execution outcome with independently owned targets.
type OperationResult struct {
	Operation  nozzle.Operation
	Status     Status
	HTTPStatus int
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
	for _, result := range results {
		urls := result.Operation.URLs
		if len(urls) == 0 {
			return results, errors.New("cloudflare: empty operation")
		}
		if len(urls) > p.config.MaxURLsPerRequest {
			return results, errors.New("cloudflare: operation exceeds the configured URL limit")
		}
		if _, err := nozzle.PlanURLs(urls, p.config.MaxURLsPerRequest); err != nil {
			return results, err
		}
	}
	if len(results) > 0 {
		if err := ctx.Err(); err != nil {
			return results, err
		}
	}
	return results, nil
}
