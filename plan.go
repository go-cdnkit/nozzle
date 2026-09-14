package nozzle

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
	targets := make([]string, len(urls))
	copy(targets, urls)
	return &Plan{Operations: []Operation{{URLs: targets}}}, nil
}
