package fastly

import (
	"context"
	"errors"
	"net"
	"net/http"
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
