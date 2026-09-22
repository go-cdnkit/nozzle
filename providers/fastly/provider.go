// Package fastly provides offline planning and sequential execution of Fastly URL purges.
package fastly

import (
	"errors"
	"net/http"
	"unicode"

	"github.com/go-cdnkit/nozzle"
)

// Config holds required Fastly settings.
type Config struct {
	// APIToken must be non-empty and contain no whitespace or control characters.
	APIToken string
	// HTTPClient is required and remains caller-owned, including its connections.
	HTTPClient *http.Client
}

// Provider executes Fastly URL purges. Construct it with New.
type Provider struct {
	config Config
}

// New validates and copies config without network I/O.
// It does not verify credentials or service eligibility.
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

// Plan calls [nozzle.PlanURLs] with one URL per operation.
func (p *Provider) Plan(urls []string) (*nozzle.Plan, error) {
	return nozzle.PlanURLs(urls, 1)
}
