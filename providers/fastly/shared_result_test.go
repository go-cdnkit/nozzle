package fastly

import (
	"context"
	"errors"
	"net"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/go-cdnkit/nozzle"
)

func TestFastlyImplementsCallerContract(t *testing.T) {
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
	var caller interface {
		Plan([]string) (*nozzle.Plan, error)
		Execute(context.Context, *nozzle.Plan) ([]nozzle.OperationResult, error)
	} = provider
	empty, err := caller.Plan(nil)
	if err != nil {
		t.Fatal(err)
	}
	results, err := caller.Execute(t.Context(), empty)
	if err != nil || results == nil || len(results) != 0 {
		t.Fatalf("empty results = %#v, error = %v", results, err)
	}
	results, err = caller.Execute(t.Context(), nil)
	if err == nil || results != nil {
		t.Fatalf("nil-plan results = %#v, error = %v", results, err)
	}
	plan, err := caller.Plan([]string{"https://example.com/a"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	results, err = caller.Execute(ctx, plan)
	if !errors.Is(err, context.Canceled) || len(results) != 1 {
		t.Fatalf("canceled results = %#v, error = %v", results, err)
	}
	if results[0].Status != nozzle.NotAttempted || results[0].HTTPStatus != 0 || !reflect.DeepEqual(results[0].Operation, plan.Operations[0]) {
		t.Fatalf("canceled result = %#v", results[0])
	}
	if calls.Load() != 0 {
		t.Fatalf("network calls = %d, want zero", calls.Load())
	}
}
