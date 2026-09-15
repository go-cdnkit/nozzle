package cloudflare

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-cdnkit/nozzle"
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

func TestCloudflarePlanUsesConfiguredLimit(t *testing.T) {
	for _, limit := range []int{1, 100, 500} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: &http.Client{}, MaxURLsPerRequest: limit})
			if err != nil {
				t.Fatal(err)
			}
			urls := make([]string, limit+1)
			for i := range urls {
				urls[i] = fmt.Sprintf("https://example.com/item/%d", i)
			}
			plan, err := provider.Plan(urls)
			if err != nil {
				t.Fatal(err)
			}
			if plan == nil || len(plan.Operations) != 2 {
				t.Fatalf("plan = %#v, want two operations", plan)
			}
			if len(plan.Operations[0].URLs) != limit || len(plan.Operations[1].URLs) != 1 {
				t.Fatalf("operation sizes = %d, %d", len(plan.Operations[0].URLs), len(plan.Operations[1].URLs))
			}
			got := append(append([]string(nil), plan.Operations[0].URLs...), plan.Operations[1].URLs...)
			if !reflect.DeepEqual(got, urls) {
				t.Fatal("planned targets differ from the input")
			}
		})
	}
}

func TestCloudflarePlanDoesNotPerformNetworkIO(t *testing.T) {
	var calls atomic.Int32
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, errors.New("network access is forbidden")
	}}
	t.Cleanup(transport.CloseIdleConnections)
	provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: &http.Client{Transport: transport}, MaxURLsPerRequest: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, urls := range [][]string{nil, {}, {"https://example.com/a"}} {
		plan, err := provider.Plan(urls)
		if err != nil || plan == nil {
			t.Fatalf("plan = %#v, error = %v", plan, err)
		}
		if len(urls) == 0 && len(plan.Operations) != 0 {
			t.Fatal("empty input produced operations")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("planning attempted network access")
	}
}

func TestCloudflarePlanRejectsInvalidInputAtomically(t *testing.T) {
	provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: &http.Client{}, MaxURLsPerRequest: 1})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := provider.Plan([]string{"https://example.com/valid", "/relative"})
	var invalid *nozzle.InvalidTargetError
	if plan != nil || !errors.As(err, &invalid) || invalid.Index != 1 {
		t.Fatalf("plan = %#v, error = %v", plan, err)
	}
}

func TestCloudflarePlanPreservesExactTargets(t *testing.T) {
	provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: &http.Client{}, MaxURLsPerRequest: 100})
	if err != nil {
		t.Fatal(err)
	}
	urls := []string{"https://EXAMPLE.com/A%2fb?b=2&a=1&a=3", "https://example.com/path?", "https://EXAMPLE.com/A%2fb?b=2&a=1&a=3"}
	plan, err := provider.Plan(urls)
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil || len(plan.Operations) != 1 || !reflect.DeepEqual(plan.Operations[0].URLs, urls) {
		t.Fatalf("plan = %#v, want unchanged targets", plan)
	}
}
