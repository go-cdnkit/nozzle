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

	if len(urls) == 0 {
		return &Plan{}, nil
	}

	targets := make([]string, len(urls))
	copy(targets, urls)
	return &Plan{Operations: []Operation{{URLs: targets}}}, nil
}
