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
	"strings"
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
				if result.Status != nozzle.NotAttempted || result.HTTPStatus != 0 || !reflect.DeepEqual(result.Operation, plan.Operations[i]) {
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
		if result.Status != nozzle.NotAttempted || result.HTTPStatus != 0 || !reflect.DeepEqual(result.Operation, plan.Operations[i]) {
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
	if len(results) != 2 || results[0].Status != nozzle.NotAttempted || results[1].Status != nozzle.NotAttempted || calls.Load() != 0 {
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
	if results[0].Status != nozzle.Accepted || results[0].HTTPStatus != http.StatusOK || !reflect.DeepEqual(results[0].Operation, plan.Operations[0]) {
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
			if err != nil || len(results) != 1 || results[0].Status != nozzle.Accepted || results[0].HTTPStatus != tt.status {
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
		if result.Status != nozzle.Accepted || result.HTTPStatus != http.StatusOK || !reflect.DeepEqual(result.Operation, plan.Operations[i]) || !reflect.DeepEqual(<-requests, plan.Operations[i].URLs) {
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
			if results[0].Status != nozzle.Rejected || results[0].HTTPStatus != status || results[1].Status != nozzle.NotAttempted || results[1].HTTPStatus != 0 {
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
			if results[0].Status != nozzle.Indeterminate || results[0].HTTPStatus != tt.status || results[1].Status != nozzle.NotAttempted {
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
	if results[0].Status != nozzle.Indeterminate || results[0].HTTPStatus != 0 || results[1].Status != nozzle.NotAttempted || results[1].HTTPStatus != 0 {
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
			want := nozzle.Rejected
			wantHTTP := http.StatusForbidden
			if !rejection {
				want = nozzle.Indeterminate
				wantHTTP = http.StatusServiceUnavailable
			}
			if results[0].Status != nozzle.Accepted || results[0].HTTPStatus != 200 || results[1].Status != want || results[1].HTTPStatus != wantHTTP || results[2].Status != nozzle.NotAttempted || results[2].HTTPStatus != 0 {
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

func TestCloudflareExecuteCancellationDuringSubmission(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var calls atomic.Int32
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		cancel()
		return nil, context.Canceled
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}
	provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
	if err != nil {
		t.Fatal(err)
	}
	plan := &nozzle.Plan{Operations: []nozzle.Operation{{URLs: []string{"https://example.com/a"}}, {URLs: []string{"https://example.com/b"}}}}
	results, err := provider.Execute(ctx, plan)
	if !errors.Is(err, context.Canceled) || len(results) != 2 || calls.Load() != 1 {
		t.Fatalf("results = %#v, error = %v, calls = %d", results, err, calls.Load())
	}
	if results[0].Status != nozzle.Indeterminate || results[1].Status != nozzle.NotAttempted {
		t.Fatalf("results = %#v", results)
	}
}

func TestCloudflareExecuteBoundsAndCompletesResponseReading(t *testing.T) {
	const limit = 64 * 1024
	base := `{"success":true}`
	tests := []struct {
		name          string
		body          string
		contentLength string
		accepted      bool
	}{
		{"at limit", base + strings.Repeat(" ", limit-len(base)), "", true},
		{"above limit", base + strings.Repeat(" ", limit-len(base)+1), "", false},
		{"truncated body", base, strconv.Itoa(len(base) + 100), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if tt.contentLength != "" {
					w.Header().Set("Content-Length", tt.contentLength)
				}
				if _, err := io.WriteString(w, tt.body); err != nil && tt.accepted {
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
			if len(results) != 1 || calls.Load() != 1 {
				t.Fatalf("results = %#v, calls = %d", results, calls.Load())
			}
			want := nozzle.Indeterminate
			if tt.accepted {
				want = nozzle.Accepted
			}
			if (err == nil) != tt.accepted || results[0].Status != want || results[0].HTTPStatus != 200 {
				t.Fatalf("results = %#v, error = %v", results, err)
			}
		})
	}
}

func TestCloudflareExecuteHonorsClientTimeout(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
			return
		}
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	client := server.Client()
	transport := client.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = "example.com"
	transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}
	client.Timeout = 5 * time.Second
	client.Timeout = 50 * time.Millisecond
	provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
	if err != nil {
		t.Fatal(err)
	}
	plan := &nozzle.Plan{Operations: []nozzle.Operation{{URLs: []string{"https://example.com/a"}}, {URLs: []string{"https://example.com/b"}}}}
	results, err := provider.Execute(t.Context(), plan)
	if !errors.Is(err, context.DeadlineExceeded) || len(results) != 2 {
		t.Fatalf("results = %#v, error = %v", results, err)
	}
	if results[0].Status != nozzle.Indeterminate || results[1].Status != nozzle.NotAttempted {
		t.Fatalf("results = %#v", results)
	}
	if client.Timeout != 50*time.Millisecond {
		t.Fatal("caller timeout was changed")
	}
}

func TestCloudflareExecuteDoesNotFollowRedirects(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(status)
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
			var redirects atomic.Int32
			redirectPolicy := func(*http.Request, []*http.Request) error {
				redirects.Add(1)
				return errors.New("caller redirect policy")
			}
			client.CheckRedirect = redirectPolicy
			provider, err := New(Config{ZoneID: "0123456789abcdef0123456789abcdef", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
			if err != nil {
				t.Fatal(err)
			}
			plan := &nozzle.Plan{Operations: []nozzle.Operation{{URLs: []string{"https://example.com/a"}}, {URLs: []string{"https://example.com/b"}}}}
			results, err := provider.Execute(t.Context(), plan)
			if err == nil || len(results) != 2 || calls.Load() != 1 || redirects.Load() != 0 {
				t.Fatalf("results = %#v, error = %v, calls = %d, redirects = %d", results, err, calls.Load(), redirects.Load())
			}
			if results[0].Status != nozzle.Indeterminate || results[0].HTTPStatus != status || results[1].Status != nozzle.NotAttempted {
				t.Fatalf("results = %#v", results)
			}
			if client.Transport != transport || client.Timeout != 5*time.Second || reflect.ValueOf(client.CheckRedirect).Pointer() != reflect.ValueOf(redirectPolicy).Pointer() {
				t.Fatal("caller HTTP client was changed")
			}
		})
	}
}

func TestCloudflareExecuteSnapshotsTargetsBeforeSending(t *testing.T) {
	plan := &nozzle.Plan{Operations: []nozzle.Operation{{URLs: []string{"https://example.com/a"}}, {URLs: []string{"https://example.com/b"}}}}
	requests := make(chan []string, 2)
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Files []string `json:"files"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if calls.Add(1) == 1 {
			plan.Operations[0].URLs[0] = "https://example.com/edited-first"
			plan.Operations[1].URLs[0] = "https://example.com/edited-second"
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
	results, err := provider.Execute(t.Context(), plan)
	if err != nil || len(results) != 2 || calls.Load() != 2 || len(requests) != 2 {
		t.Fatalf("results = %#v, error = %v", results, err)
	}
	want := [][]string{{"https://example.com/a"}, {"https://example.com/b"}}
	for i, result := range results {
		if result.Status != nozzle.Accepted || !reflect.DeepEqual(result.Operation.URLs, want[i]) || !reflect.DeepEqual(<-requests, want[i]) {
			t.Fatalf("operation %d differs: %#v", i, result)
		}
	}
	results[0].Operation.URLs[0] = "https://example.com/result-edit"
	if plan.Operations[0].URLs[0] != "https://example.com/edited-first" || results[1].Operation.URLs[0] != "https://example.com/b" {
		t.Fatal("result storage aliases caller or another result")
	}
}

func TestCloudflareExecuteDoesNotEchoRemoteResponseBodies(t *testing.T) {
	const secret = "private-response-marker"
	for _, body := range []string{
		`{"success":false,"errors":[{"code":1000,"message":"private-response-marker test-token"}]}`,
		"<html>private-response-marker test-token</html>",
	} {
		t.Run(body[:1], func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			plan := &nozzle.Plan{Operations: []nozzle.Operation{{URLs: []string{"https://example.com/a?secret=private-response-marker"}}}}
			results, err := provider.Execute(t.Context(), plan)
			if err == nil || len(results) != 1 {
				t.Fatalf("results = %#v, error = %v", results, err)
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "test-token") {
				t.Fatal("error includes response or credential data")
			}
		})
	}
}
