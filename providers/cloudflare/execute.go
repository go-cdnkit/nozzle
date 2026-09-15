package cloudflare

import (
	"context"
	"errors"

	"github.com/go-cdnkit/nozzle"
)

// OperationResult associates an execution outcome with its exact targets.
type OperationResult struct {
	Operation nozzle.Operation
}

// Execute returns no results for an empty plan.
func (p *Provider) Execute(ctx context.Context, plan *nozzle.Plan) ([]OperationResult, error) {
	if plan == nil {
		return nil, errors.New("cloudflare: nil execution plan")
	}
	return nil, nil
}
