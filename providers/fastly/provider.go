// Package fastly provides offline planning and sequential execution of Fastly URL purges.
package fastly

import (
	"errors"
	"net/http"
	"unicode"

	"github.com/go-cdnkit/nozzle"
)

// Config holds explicit Fastly configuration. No values are discovered or defaulted.
type Config struct {
	// APIToken must be non-empty and contain no whitespace or control characters.
	APIToken string
	// HTTPClient is required and remains caller-owned, including its connections.
	HTTPClient *http.Client
}

// Provider holds a copy of its configuration. Construct it with New.
type Provider struct {
	config Config
}

// New validates configuration and constructs a provider without network I/O.
// It does not verify credentials or service eligibility. Errors omit credentials.
// The HTTP client remains caller-owned and is neither cloned nor modified.
func New(config Config) (*Provider, error) {
	if config.APIToken == "" {
		return nil, errors.New("fastly: empty API token")
	}
	for _, r := range config.APIToken {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return nil, errors.New("fastly: API token contains whitespace or a control character")
		}
	}
	if config.HTTPClient == nil {
		return nil, errors.New("fastly: nil HTTP client")
	}

	return &Provider{config: config}, nil
}

// Plan validates exact URLs and creates one operation per URL without network I/O.
// It preserves the input and error semantics of nozzle.PlanURLs, including duplicates.
// The returned plan owns its URL slices but remains editable and is not bound to
// this provider. Planning does not verify Fastly routing or cache-key coverage.
func (p *Provider) Plan(urls []string) (*nozzle.Plan, error) {
	return nozzle.PlanURLs(urls, 1)
}
