package fastly

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

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

func TestFastlyExecuteRejectsNilPlan(t *testing.T) {
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
	results, err := provider.Execute(t.Context(), nil)
	if err == nil || len(results) != 0 || calls.Load() != 0 {
		t.Fatalf("results = %#v, error = %v, calls = %d", results, err, calls.Load())
	}
}

func TestFastlyExecuteValidatesWholePlanBeforeSending(t *testing.T) {
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
	tests := []struct {
		name      string
		operation nozzle.Operation
	}{
		{"empty operation", nozzle.Operation{}},
		{"over capacity", nozzle.Operation{URLs: []string{"https://example.com/a", "https://example.com/b"}}},
		{"relative URL", nozzle.Operation{URLs: []string{"/relative"}}},
		{"fragment", nozzle.Operation{URLs: []string{"https://example.com/a#fragment"}}},
		{"credentials", nozzle.Operation{URLs: []string{"https://user:secret@example.com/a"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &nozzle.Plan{Operations: []nozzle.Operation{{URLs: []string{"https://example.com/valid"}}, tt.operation}}
			results, err := provider.Execute(t.Context(), plan)
			if err == nil || len(results) != 2 {
				t.Fatalf("results = %#v, error = %v", results, err)
			}
			for i, result := range results {
				if result.Status != NotAttempted || result.HTTPStatus != 0 || !reflect.DeepEqual(result.Operation, plan.Operations[i]) {
					t.Fatalf("result %d = %#v", i, result)
				}
			}
			if calls.Load() != 0 {
				t.Fatal("invalid plan performed network I/O")
			}
		})
	}
}

func TestFastlyExecuteHonorsPreCanceledContext(t *testing.T) {
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
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	plan := &nozzle.Plan{Operations: []nozzle.Operation{{URLs: []string{"https://example.com/a"}}, {URLs: []string{"https://example.com/b"}}}}
	results, err := provider.Execute(ctx, plan)
	if !errors.Is(err, context.Canceled) || len(results) != 2 || calls.Load() != 0 {
		t.Fatalf("results = %#v, error = %v, calls = %d", results, err, calls.Load())
	}
	for i, result := range results {
		if result.Status != NotAttempted || result.HTTPStatus != 0 || !reflect.DeepEqual(result.Operation, plan.Operations[i]) {
			t.Fatalf("result %d = %#v", i, result)
		}
	}
}

func TestFastlyExecutePreservesPreflightErrorLocation(t *testing.T) {
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
	plan := &nozzle.Plan{Operations: []nozzle.Operation{{URLs: []string{"https://example.com/valid"}}, {URLs: []string{"/relative"}}}}
	results, err := provider.Execute(t.Context(), plan)
	var operationErr *OperationError
	var invalid *nozzle.InvalidTargetError
	if !errors.As(err, &operationErr) || operationErr.Index != 1 || !errors.As(err, &invalid) || invalid.Index != 0 {
		t.Fatalf("error = %v", err)
	}
	if len(results) != 2 || results[0].Status != NotAttempted || results[1].Status != NotAttempted || calls.Load() != 0 {
		t.Fatalf("results = %#v, calls = %d", results, calls.Load())
	}
}

func TestFastlyExecuteSendsExactURLRequests(t *testing.T) {
	urls := []string{
		"https://EXAMPLE.com/A%2fb?b=2&a=1&a=3",
		"https://example.com/path?",
		"http://example.com/a?x=a+b&x=a%20b&empty=",
		"https://example.com/a%3Fb%23c?x=%2f&x=%2F",
		"https://example.com/a/../b//c",
		"https://example.com",
		"https://example.com:8443/a",
		"https://EXAMPLE.com/A%2fb?b=2&a=1&a=3",
	}
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(calls.Add(1)) - 1
		if i >= len(urls) {
			t.Error("extra request")
			return
		}
		if r.Method != http.MethodPost || r.Host != "api.fastly.com" || r.TLS == nil || r.RequestURI != "/purge/"+urls[i] {
			t.Errorf("unexpected request: %s %s %s", r.Method, r.Host, r.RequestURI)
		}
		if r.Header.Get("Fastly-Key") != "test-token" || r.Header.Get("Accept") != "application/json" {
			t.Error("authentication or accept header differs")
		}
		for _, name := range []string{"Authorization", "Fastly-Soft-Purge", "Surrogate-Key", "Idempotency-Key", "X-Idempotency-Key"} {
			if r.Header.Get(name) != "" {
				t.Errorf("unexpected header %s", name)
			}
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || len(body) != 0 {
			t.Errorf("body = %q, error = %v", body, err)
		}
		if _, err := io.WriteString(w, `{"status":"ok","id":"purge-id"}`); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	client := server.Client()
	transport := client.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = "example.com"
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != "api.fastly.com:443" {
			return nil, errors.New("unexpected dial destination")
		}
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}
	client.Timeout = 5 * time.Second
	provider, err := New(Config{APIToken: "test-token", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := provider.Plan(urls)
	if err != nil {
		t.Fatal(err)
	}
	results, err := provider.Execute(t.Context(), plan)
	if err != nil || len(results) != len(urls) || int(calls.Load()) != len(urls) {
		t.Fatalf("results = %#v, error = %v, calls = %d", results, err, calls.Load())
	}
	for i, result := range results {
		if result.Status != Accepted || result.HTTPStatus != 200 || !reflect.DeepEqual(result.Operation, plan.Operations[i]) {
			t.Fatalf("result %d = %#v", i, result)
		}
	}
}
