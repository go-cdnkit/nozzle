// Package fastly provides offline planning for Fastly URL purges.
package fastly

import (
	"errors"
	"net/http"
	"unicode"
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
