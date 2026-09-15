package cloudflare

import (
	"context"
	"errors"
	"net"
	"net/http"
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
