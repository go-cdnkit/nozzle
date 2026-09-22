package fastly

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/go-cdnkit/nozzle"
)

func TestFastlyExecuteEmptyPlan(t *testing.T) {
	var calls atomic.Int32
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, errors.New("network access is forbidden")
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}
	provider, err := New(Config{APIToken: "test-token", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	results, err := provider.Execute(t.Context(), &nozzle.Plan{})
	if err != nil || len(results) != 0 || calls.Load() != 0 {
		t.Fatalf("results = %#v, error = %v, calls = %d", results, err, calls.Load())
	}
}
