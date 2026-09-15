package cloudflare

import (
	"context"

	"github.com/go-cdnkit/nozzle"
)

// OperationResult associates an execution outcome with its exact targets.
type OperationResult struct {
	Operation nozzle.Operation
}

// Execute returns no results for an empty plan.
func (p *Provider) Execute(ctx context.Context, plan *nozzle.Plan) ([]OperationResult, error) {
	return nil, nil
}
