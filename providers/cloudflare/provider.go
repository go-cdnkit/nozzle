// Package cloudflare provides offline planning and sequential execution of Cloudflare URL purges.
package cloudflare

import (
	"errors"
	"net/http"
	"unicode"

	"github.com/go-cdnkit/nozzle"
)

// Config holds required Cloudflare settings.
type Config struct {
	// ZoneID is a non-empty ASCII alphanumeric identifier of at most 32 bytes.
	ZoneID string
	// APIToken must be non-empty and contain no whitespace or control characters.
	APIToken string
	// HTTPClient is required and remains caller-owned, including its connections.
	HTTPClient *http.Client
	// MaxURLsPerRequest is an explicit capacity from 1 through 500.
	// The caller must choose a value supported by its account; New cannot verify it.
	MaxURLsPerRequest int
}

// Provider executes Cloudflare URL purges. Construct it with New.
type Provider struct {
	config Config
}

// New validates and copies config without network I/O.
// It does not verify credentials, zone ownership or account request limits.
func New(config Config) (*Provider, error) {
	if len(config.ZoneID) == 0 || len(config.ZoneID) > 32 {
		return nil, errors.New("cloudflare: zone ID length is outside 1..32 bytes")
	}
	for _, r := range config.ZoneID {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return nil, errors.New("cloudflare: zone ID contains a character outside ASCII letters and digits")
		}
	}
	if config.APIToken == "" {
		return nil, errors.New("cloudflare: empty API token")
	}
	for _, r := range config.APIToken {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return nil, errors.New("cloudflare: API token contains whitespace or a control character")
		}
	}
	if config.HTTPClient == nil {
		return nil, errors.New("cloudflare: nil HTTP client")
	}
	if config.MaxURLsPerRequest < 1 || config.MaxURLsPerRequest > 500 {
		return nil, errors.New("cloudflare: URL request limit is outside 1..500")
	}

	return &Provider{config: config}, nil
}

// Plan calls [nozzle.PlanURLs] with the configured MaxURLsPerRequest.
func (p *Provider) Plan(urls []string) (*nozzle.Plan, error) {
	return nozzle.PlanURLs(urls, p.config.MaxURLsPerRequest)
}
