package nozzle

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

// Plan describes URL operations without performing network requests.
// Its fields are mutable; it is not a provider-bound execution authorization.
type Plan struct {
	Operations []Operation
}

// Operation groups exact URLs under the supplied request-size limit.
// Provider-specific eligibility is not established by this grouping.
type Operation struct {
	URLs []string
}

// InvalidTargetError identifies the first invalid input without echoing its URL.
type InvalidTargetError struct {
	// Index is the zero-based position in the input slice.
	Index int
	// Reason describes the validation failure, not the supplied URL.
	Reason string
}

func (e *InvalidTargetError) Error() string {
	return fmt.Sprintf("invalid target at index %d: %s", e.Index, e.Reason)
}

// PlanURLs validates and groups exact HTTP/HTTPS URLs without network I/O.
// It preserves URL text, input order and duplicate occurrences. Each operation
// owns a copy of its URLs, independent of the input and other operations.
//
// maxURLsPerOperation must be positive, even for empty input. Empty input returns
// a non-nil plan with no operations. Invalid input returns no plan and an
// *InvalidTargetError identifying the first invalid input. URL parsing uses
// net/url; missing hosts, whitespace, user information and fragments are rejected.
//
// The caller supplies a limit appropriate for its provider. Planning does not
// resolve hosts, check provider support or credentials, or invalidate any cache.
func PlanURLs(urls []string, maxURLsPerOperation int) (*Plan, error) {
	if maxURLsPerOperation <= 0 {
		return nil, fmt.Errorf("non-positive URL limit: %d", maxURLsPerOperation)
	}

	for i, target := range urls {
		if strings.IndexFunc(target, unicode.IsSpace) >= 0 {
			return nil, &InvalidTargetError{Index: i, Reason: "URL contains whitespace"}
		}
		parsed, err := url.Parse(target)
		if err != nil {
			// Parse errors may include the supplied URL, including secrets.
			return nil, &InvalidTargetError{Index: i, Reason: "malformed URL"}
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return nil, &InvalidTargetError{Index: i, Reason: "unsupported or missing scheme"}
		}
		if parsed.Hostname() == "" {
			return nil, &InvalidTargetError{Index: i, Reason: "missing host"}
		}
		if parsed.User != nil {
			return nil, &InvalidTargetError{Index: i, Reason: "URL contains user information"}
		}
		// Inspect the original text so an empty fragment is rejected as well.
		if strings.Contains(target, "#") {
			return nil, &InvalidTargetError{Index: i, Reason: "URL contains a fragment"}
		}
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
