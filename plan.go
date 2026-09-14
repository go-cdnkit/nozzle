package nozzle

import "fmt"

// Plan describes URL operations without performing network requests.
type Plan struct {
	Operations []Operation
}

// Operation groups exact URLs for one provider request.
type Operation struct {
	URLs []string
}

// PlanURLs builds a plan from exact URLs.
func PlanURLs(urls []string, maxURLsPerOperation int) (*Plan, error) {
	if maxURLsPerOperation <= 0 {
		return nil, fmt.Errorf("non-positive URL limit: %d", maxURLsPerOperation)
	}

	plan := &Plan{}
	for start := 0; start < len(urls); {
		size := min(maxURLsPerOperation, len(urls)-start)
		targets := make([]string, size)
		copy(targets, urls[start:start+size])
		plan.Operations = append(plan.Operations, Operation{URLs: targets})
		start += size
	}
	return plan, nil
}
