// Package cloudflare provides offline planning for Cloudflare URL purges.
package cloudflare

import "net/http"

// Config holds explicit Cloudflare configuration. No values are discovered or defaulted.
type Config struct {
	ZoneID            string
	APIToken          string
	HTTPClient        *http.Client
	MaxURLsPerRequest int
}

// Provider holds a copy of its configuration. Construct it with New.
type Provider struct {
	config Config
}

// New constructs a provider without network I/O.
// The HTTP client remains caller-owned and is neither cloned nor modified.
func New(config Config) (*Provider, error) {
	return &Provider{config: config}, nil
}
