package fastly

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
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

func TestFastlyExecuteAcceptsOptionalMetadata(t *testing.T) {
	tests := []struct {
		status int
		body   string
	}{
		{200, `{"status":"ok"}`},
		{201, `{"status":"ok","id":null,"msg":null,"detail":"","errors":[]}`},
		{202, `{"status":"ok","id":"purge-id","extra":{"future":"value"}}`},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				if _, err := io.WriteString(w, tt.body); err != nil {
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
			plan := &nozzle.Plan{Operations: []nozzle.Operation{{URLs: []string{"https://example.com/a"}}}}
			results, err := provider.Execute(t.Context(), plan)
			if err != nil || len(results) != 1 || results[0].Status != Accepted || results[0].HTTPStatus != tt.status {
				t.Fatalf("results = %#v, error = %v", results, err)
			}
		})
	}
}

func TestFastlyExecutePreservesOperationOrder(t *testing.T) {
	requests := make(chan string, 3)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requests <- r.RequestURI:
		default:
			t.Error("extra request")
		}
		if _, err := io.WriteString(w, `{"status":"ok"}`); err != nil {
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
	plan := &nozzle.Plan{Operations: []nozzle.Operation{
		{URLs: []string{"https://example.com/a"}},
		{URLs: []string{"https://example.com/a"}},
		{URLs: []string{"https://example.com/c"}},
	}}
	results, err := provider.Execute(t.Context(), plan)
	if err != nil || len(results) != 3 || len(requests) != 3 {
		t.Fatalf("results = %#v, error = %v, requests = %d", results, err, len(requests))
	}
	for i, result := range results {
		if result.Status != Accepted || result.HTTPStatus != http.StatusOK || !reflect.DeepEqual(result.Operation, plan.Operations[i]) || <-requests != "/purge/"+plan.Operations[i].URLs[0] {
			t.Fatalf("operation %d differs", i)
		}
	}
}

func TestFastlyExecuteReportsExplicitRejection(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusTooManyRequests} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(status)
				if _, err := io.WriteString(w, `{"msg":"rejected","detail":"permission or quota failure"}`); err != nil {
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
			plan := &nozzle.Plan{Operations: []nozzle.Operation{{URLs: []string{"https://example.com/a"}}, {URLs: []string{"https://example.com/b"}}}}
			results, err := provider.Execute(t.Context(), plan)
			if err == nil || len(results) != 2 || calls.Load() != 1 {
				t.Fatalf("results = %#v, error = %v, calls = %d", results, err, calls.Load())
			}
			if results[0].Status != Rejected || results[0].HTTPStatus != status || results[1].Status != NotAttempted || results[1].HTTPStatus != 0 {
				t.Fatalf("results = %#v", results)
			}
		})
	}
}

func TestFastlyExecuteDoesNotGuessAcceptance(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"empty", 200, ""},
		{"HTML", 200, "<html>error</html>"},
		{"missing status", 200, "{}"},
		{"null status", 200, "{\"status\":null}"},
		{"wrong type", 200, "{\"status\":true}"},
		{"unknown status", 200, "{\"status\":\"success\"}"},
		{"case sensitive", 200, "{\"status\":\"OK\"}"},
		{"trailing JSON", 200, "{\"status\":\"ok\"}{}"},
		{"trailing garbage", 200, "{\"status\":\"ok\"}broken"},
		{"duplicate status", 200, "{\"status\":\"error\",\"status\":\"ok\"}"},
		{"escaped duplicate", 200, "{\"status\":\"ok\",\"sta\\u0074us\":\"ok\"}"},
		{"contradictory msg", 200, "{\"status\":\"ok\",\"msg\":\"failure\"}"},
		{"contradictory detail", 200, "{\"status\":\"ok\",\"detail\":\"failure\"}"},
		{"wrong msg type", 200, "{\"status\":\"ok\",\"msg\":42}"},
		{"duplicate msg", 403, "{\"msg\":\"denied\",\"msg\":\"denied\"}"},
		{"wrong detail type", 403, "{\"msg\":\"denied\",\"detail\":{}}"},
		{"server error", 500, "{\"msg\":\"rejected\"}"},
		{"server positive", 503, "{\"status\":\"ok\"}"},
		{"client positive", 403, "{\"status\":\"ok\"}"},
		{"rate limit without envelope", 429, ""},
		{"request timeout", 408, "{\"msg\":\"rejected\"}"},
		{"redirect", 302, "{\"status\":\"ok\"}"},
		{"JSON API error", 403, "{\"errors\":[{\"title\":\"forbidden\"}]}"},
		{"problem detail", 403, "{\"status\":403,\"title\":\"forbidden\"}"},
		{"nonobject", 200, "[{\"status\":\"ok\"}]"},
		{"duplicate detail", 200, "{\"status\":\"ok\",\"detail\":\"\",\"detail\":\"\"}"},
		{"contradictory errors", 200, `{"status":"ok","errors":[{"title":"failure"}]}`},
		{"malformed errors", 200, `{"status":"ok","errors":"failure"}`},
		{"duplicate errors", 200, `{"status":"ok","errors":[],"errors":[]}`},
		{"empty negative", 403, "{\"msg\":\"\"}"},
		{"negative in 2xx", 200, "{\"msg\":\"rejected\"}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(tt.status)
				if _, err := io.WriteString(w, tt.body); err != nil {
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
			plan := &nozzle.Plan{Operations: []nozzle.Operation{{URLs: []string{"https://example.com/a"}}, {URLs: []string{"https://example.com/b"}}}}
			results, err := provider.Execute(t.Context(), plan)
			if err == nil || len(results) != 2 || calls.Load() != 1 {
				t.Fatalf("results = %#v, error = %v, calls = %d", results, err, calls.Load())
			}
			if results[0].Status != Indeterminate || results[0].HTTPStatus != tt.status || results[1].Status != NotAttempted {
				t.Fatalf("results = %#v", results)
			}
		})
	}
}

func TestFastlyExecutePreservesTransportError(t *testing.T) {
	sentinel := errors.New("transport failure")
	var calls atomic.Int32
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, sentinel
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}
	provider, err := New(Config{APIToken: "test-token", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	plan := &nozzle.Plan{Operations: []nozzle.Operation{{URLs: []string{"https://example.com/a"}}, {URLs: []string{"https://example.com/b"}}}}
	results, err := provider.Execute(t.Context(), plan)
	if !errors.Is(err, sentinel) || len(results) != 2 || calls.Load() != 1 {
		t.Fatalf("results = %#v, error = %v, calls = %d", results, err, calls.Load())
	}
	if results[0].Status != Indeterminate || results[0].HTTPStatus != 0 || results[1].Status != NotAttempted || results[1].HTTPStatus != 0 {
		t.Fatalf("results = %#v", results)
	}
}
