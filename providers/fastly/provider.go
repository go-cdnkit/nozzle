// Package fastly provides offline planning for Fastly URL purges.
package fastly

import "net/http"

// Config holds explicit Fastly configuration. No values are discovered or defaulted.
type Config struct {
	APIToken   string
	HTTPClient *http.Client
}

// Provider holds a copy of its configuration. Construct it with New.
type Provider struct {
	config Config
}

// New constructs a provider without network I/O.
func New(config Config) (*Provider, error) {
	return &Provider{config: config}, nil
}
