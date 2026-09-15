package cloudflare

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNewValidCloudflareConfiguration(t *testing.T) {
	var calls atomic.Int32
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, errors.New("network access is forbidden")
	}}
	t.Cleanup(transport.CloseIdleConnections)
	provider, err := New(Config{
		ZoneID:            "0123456789abcdef0123456789abcdef",
		APIToken:          "test-token",
		HTTPClient:        &http.Client{Transport: transport},
		MaxURLsPerRequest: 100,
	})
	if err != nil || provider == nil {
		t.Fatalf("provider = %v, error = %v", provider, err)
	}
	if calls.Load() != 0 {
		t.Fatal("construction attempted network access")
	}
}

func TestNewRejectsInvalidCloudflareConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Config)
	}{
		{"empty zone", func(c *Config) { c.ZoneID = "" }},
		{"zone path", func(c *Config) { c.ZoneID = "../zones/other" }},
		{"zone whitespace", func(c *Config) { c.ZoneID = " " }},
		{"zone too long", func(c *Config) { c.ZoneID = strings.Repeat("a", 33) }},
		{"empty token", func(c *Config) { c.APIToken = "" }},
		{"token whitespace", func(c *Config) { c.APIToken = "test token" }},
		{"token newline", func(c *Config) { c.APIToken = "test\r\nInjected: value" }},
		{"nil client", func(c *Config) { c.HTTPClient = nil }},
		{"zero limit", func(c *Config) { c.MaxURLsPerRequest = 0 }},
		{"negative limit", func(c *Config) { c.MaxURLsPerRequest = -1 }},
		{"above supported limit", func(c *Config) { c.MaxURLsPerRequest = 501 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: &http.Client{}, MaxURLsPerRequest: 100}
			tt.change(&config)
			provider, err := New(config)
			if err == nil || provider != nil {
				t.Fatalf("provider = %v, error = %v", provider, err)
			}
		})
	}
}
