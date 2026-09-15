package cloudflare

import (
	"context"
	"encoding/json"
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

func TestCloudflareExecuteEmptyPlan(t *testing.T) {
	var calls atomic.Int32
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, errors.New("network access is forbidden")
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}
	provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
	if err != nil {
		t.Fatal(err)
	}
	results, err := provider.Execute(t.Context(), &nozzle.Plan{})
	if err != nil || len(results) != 0 || calls.Load() != 0 {
		t.Fatalf("results = %#v, error = %v, calls = %d", results, err, calls.Load())
	}
}

func TestCloudflareExecuteRejectsNilPlan(t *testing.T) {
	var calls atomic.Int32
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, errors.New("network access is forbidden")
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}
	provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
	if err != nil {
		t.Fatal(err)
	}
	results, err := provider.Execute(t.Context(), nil)
	if err == nil || len(results) != 0 || calls.Load() != 0 {
		t.Fatalf("results = %#v, error = %v, calls = %d", results, err, calls.Load())
	}
}

func TestCloudflareExecuteValidatesWholePlanBeforeSending(t *testing.T) {
	var calls atomic.Int32
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, errors.New("network access is forbidden")
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}
	provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		operation nozzle.Operation
	}{
		{"empty operation", nozzle.Operation{}},
		{"over capacity", nozzle.Operation{URLs: []string{"https://example.com/a", "https://example.com/b", "https://example.com/c"}}},
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

func TestCloudflareExecuteHonorsPreCanceledContext(t *testing.T) {
	var calls atomic.Int32
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, errors.New("network access is forbidden")
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}
	provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
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

func TestCloudflareExecutePreservesPreflightErrorLocation(t *testing.T) {
	var calls atomic.Int32
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, errors.New("network access is forbidden")
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}
	provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
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

func TestCloudflareExecuteSendsExactURLBatch(t *testing.T) {
	urls := []string{"https://EXAMPLE.com/A%2fb?b=2&a=1&a=3", "https://example.com/path?"}
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.Host != "api.cloudflare.com" || r.URL.Path != "/client/v4/zones/0123456789abcdef0123456789abcdef/purge_cache" || r.URL.RawQuery != "" {
			t.Errorf("unexpected request: %s %s %s", r.Method, r.Host, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("bearer token differs")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("content type differs")
		}
		if r.Header.Get("Idempotency-Key") != "" || r.Header.Get("X-Idempotency-Key") != "" {
			t.Error("unexpected retry-enabling header")
		}
		var body map[string][]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body) != 1 || !reflect.DeepEqual(body["files"], urls) {
			t.Errorf("body = %#v", body)
		}
		if _, err := io.WriteString(w, `{"success":true,"errors":[],"result":{"id":"operation-id"}}`); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	client := server.Client()
	transport := client.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = "example.com"
	transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}
	client.Timeout = 5 * time.Second
	provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := provider.Plan(urls)
	if err != nil {
		t.Fatal(err)
	}
	results, err := provider.Execute(t.Context(), plan)
	if err != nil || len(results) != 1 || calls.Load() != 1 {
		t.Fatalf("results = %#v, error = %v, calls = %d", results, err, calls.Load())
	}
	if results[0].Status != Accepted || results[0].HTTPStatus != http.StatusOK || !reflect.DeepEqual(results[0].Operation, plan.Operations[0]) {
		t.Fatalf("result = %#v", results[0])
	}
}

func TestCloudflareExecuteAcceptsOptionalMetadata(t *testing.T) {
	tests := []struct {
		status int
		body   string
	}{
		{200, `{"success":true}`},
		{201, `{"success":true,"result":null,"errors":[]}`},
		{202, `{"success":true,"errors":[],"messages":[{"code":1000,"message":"notice"}],"extra":{"future":"value"}}`},
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
			transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
			}
			client.Timeout = 5 * time.Second
			provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
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

func TestCloudflareExecutePreservesOperationOrder(t *testing.T) {
	requests := make(chan []string, 3)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Files []string `json:"files"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		select {
		case requests <- body.Files:
		default:
			t.Error("extra request")
		}
		if _, err := io.WriteString(w, `{"success":true}`); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	client := server.Client()
	transport := client.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = "example.com"
	transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}
	client.Timeout = 5 * time.Second
	provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
	if err != nil {
		t.Fatal(err)
	}
	plan := &nozzle.Plan{Operations: []nozzle.Operation{
		{URLs: []string{"https://example.com/a"}},
		{URLs: []string{"https://example.com/b", "https://example.com/b"}},
		{URLs: []string{"https://example.com/c"}},
	}}
	results, err := provider.Execute(t.Context(), plan)
	if err != nil || len(results) != 3 || len(requests) != 3 {
		t.Fatalf("results = %#v, error = %v, requests = %d", results, err, len(requests))
	}
	for i, result := range results {
		if result.Status != Accepted || result.HTTPStatus != http.StatusOK || !reflect.DeepEqual(result.Operation, plan.Operations[i]) || !reflect.DeepEqual(<-requests, plan.Operations[i].URLs) {
			t.Fatalf("operation %d differs", i)
		}
	}
}

func TestCloudflareExecuteReportsExplicitRejection(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusBadRequest, http.StatusForbidden, http.StatusTooManyRequests} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(status)
				if _, err := io.WriteString(w, `{"success":false,"errors":[{"code":1000,"message":"rejected"}]}`); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			client := server.Client()
			transport := client.Transport.(*http.Transport)
			transport.TLSClientConfig.ServerName = "example.com"
			transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
			}
			client.Timeout = 5 * time.Second
			provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
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

func TestCloudflareExecuteDoesNotGuessAcceptance(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"empty", 200, ""},
		{"HTML", 200, "<html>error</html>"},
		{"missing success", 200, "{}"},
		{"null success", 200, `{"success":null}`},
		{"string success", 200, `{"success":"true"}`},
		{"trailing JSON", 200, `{"success":true}{"success":false}`},
		{"trailing garbage", 200, `{"success":true}broken`},
		{"duplicate success", 200, `{"success":false,"success":true}`},
		{"contradictory errors", 200, `{"success":true,"errors":[{"code":1000,"message":"failure"}]}`},
		{"malformed errors", 200, `{"success":true,"errors":"failure"}`},
		{"server error with false", 500, `{"success":false}`},
		{"server error with true", 503, `{"success":true}`},
		{"client error with true", 403, `{"success":true}`},
		{"rate limit without envelope", 429, ""},
		{"request timeout", 408, `{"success":false}`},
		{"redirect without location", 302, `{"success":true}`},
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
			transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
			}
			client.Timeout = 5 * time.Second
			provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
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

func TestCloudflareExecutePreservesTransportError(t *testing.T) {
	sentinel := errors.New("transport failure")
	var calls atomic.Int32
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, sentinel
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}
	provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
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

func TestCloudflareExecuteRetainsPartialResults(t *testing.T) {
	for _, rejection := range []bool{true, false} {
		t.Run(strconv.FormatBool(rejection), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				body := `{"success":true}`
				if n == 2 {
					if rejection {
						w.WriteHeader(http.StatusForbidden)
						body = `{"success":false}`
					} else {
						w.WriteHeader(http.StatusServiceUnavailable)
						body = "<html>unavailable</html>"
					}
				}
				if _, err := io.WriteString(w, body); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			client := server.Client()
			transport := client.Transport.(*http.Transport)
			transport.TLSClientConfig.ServerName = "example.com"
			transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
			}
			client.Timeout = 5 * time.Second
			provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
			if err != nil {
				t.Fatal(err)
			}
			plan := &nozzle.Plan{Operations: []nozzle.Operation{{URLs: []string{"https://example.com/a"}}, {URLs: []string{"https://example.com/b"}}, {URLs: []string{"https://example.com/c"}}}}
			results, err := provider.Execute(t.Context(), plan)
			if err == nil || len(results) != 3 || calls.Load() != 2 {
				t.Fatalf("results = %#v, error = %v, calls = %d", results, err, calls.Load())
			}
			want := Rejected
			wantHTTP := http.StatusForbidden
			if !rejection {
				want = Indeterminate
				wantHTTP = http.StatusServiceUnavailable
			}
			if results[0].Status != Accepted || results[0].HTTPStatus != 200 || results[1].Status != want || results[1].HTTPStatus != wantHTTP || results[2].Status != NotAttempted || results[2].HTTPStatus != 0 {
				t.Fatalf("results = %#v", results)
			}
			for i, result := range results {
				if !reflect.DeepEqual(result.Operation, plan.Operations[i]) {
					t.Fatalf("result %d lost targets", i)
				}
			}
		})
	}
}
