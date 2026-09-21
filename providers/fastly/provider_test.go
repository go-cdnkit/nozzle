package fastly

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNewValidFastlyConfiguration(t *testing.T) {
	var calls atomic.Int32
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, errors.New("network access is forbidden")
	}}
	t.Cleanup(transport.CloseIdleConnections)
	provider, err := New(Config{APIToken: "test-token", HTTPClient: &http.Client{Transport: transport}})
	if err != nil || provider == nil {
		t.Fatalf("provider = %v, error = %v", provider, err)
	}
	if calls.Load() != 0 {
		t.Fatal("construction attempted network access")
	}
}

func TestNewRejectsInvalidFastlyConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		token  string
		client *http.Client
	}{
		{"empty token", "", &http.Client{}},
		{"space", "test token", &http.Client{}},
		{"tab", "test\ttoken", &http.Client{}},
		{"newline", "test\r\nInjected: value", &http.Client{}},
		{"control", "test\x00token", &http.Client{}},
		{"unicode whitespace", "test\u00a0token", &http.Client{}},
		{"nil client", "test-token", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := New(Config{APIToken: tt.token, HTTPClient: tt.client})
			if err == nil || provider != nil {
				t.Fatalf("provider = %v, error = %v", provider, err)
			}
		})
	}
}

func TestFastlyConfigurationErrorsDoNotEchoCredentials(t *testing.T) {
	const secret = "private-token-marker"
	for _, config := range []Config{
		{APIToken: secret + "\n", HTTPClient: &http.Client{}},
		{APIToken: secret, HTTPClient: nil},
	} {
		provider, err := New(config)
		if err == nil || provider != nil {
			t.Fatal("invalid configuration was accepted")
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatal("configuration error exposes credentials")
		}
	}
}

func TestFastlyPlanUsesOneURLPerOperation(t *testing.T) {
	provider, err := New(Config{APIToken: "test-token", HTTPClient: &http.Client{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{1, 2, 501} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			urls := make([]string, count)
			for i := range urls {
				urls[i] = "https://example.com/item/" + strconv.Itoa(i)
			}
			plan, err := provider.Plan(urls)
			if err != nil || plan == nil || len(plan.Operations) != count {
				t.Fatalf("plan = %#v, error = %v", plan, err)
			}
			for i, operation := range plan.Operations {
				if len(operation.URLs) != 1 || operation.URLs[0] != urls[i] {
					t.Fatalf("operation %d = %#v", i, operation)
				}
			}
		})
	}
}
