package cloudflare

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

func TestCloudflareImplementsCallerContract(t *testing.T) {
	var calls atomic.Int32
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, errors.New("network access is forbidden")
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}
	provider, err := New(Config{ZoneID: "testzone", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
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

func TestCloudflareSharedResultsPreservePartialProgress(t *testing.T) {
	for _, tt := range []struct {
		name   string
		http   int
		status nozzle.Status
	}{
		{"rejected", http.StatusForbidden, nozzle.Rejected},
		{"indeterminate", http.StatusBadGateway, nozzle.Indeterminate},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, err := io.Copy(io.Discard, r.Body); err != nil {
					t.Error(err)
				}
				body := `{"success":true}`
				switch calls.Add(1) {
				case 1:
				case 2:
					w.WriteHeader(tt.http)
					body = `{"success":false}`
				default:
					t.Error("extra request after partial failure")
					w.WriteHeader(http.StatusInternalServerError)
				}
				if _, err := io.WriteString(w, body); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			client := server.Client()
			transport := client.Transport.(*http.Transport)
			transport.TLSClientConfig.ServerName = "example.com"
			transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				if network != "tcp" || address != "api.cloudflare.com:443" {
					return nil, errors.New("unexpected dial destination")
				}
				return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
			}
			client.Timeout = 5 * time.Second
			provider, err := New(Config{ZoneID: "testzone", APIToken: "test-token", HTTPClient: client, MaxURLsPerRequest: 2})
			if err != nil {
				t.Fatal(err)
			}
			var caller interface {
				Plan([]string) (*nozzle.Plan, error)
				Execute(context.Context, *nozzle.Plan) ([]nozzle.OperationResult, error)
			} = provider
			urls := []string{
				"https://example.com/A%2fb?b=2&a=1",
				"https://example.com/b",
				"https://example.com/c",
				"https://example.com/d",
				"https://example.com/A%2fb?b=2&a=1",
			}
			plan, err := caller.Plan(urls)
			if err != nil {
				t.Fatal(err)
			}
			results, err := caller.Execute(t.Context(), plan)
			var operationErr *OperationError
			if !errors.As(err, &operationErr) || operationErr.Index != 1 {
				t.Fatalf("error = %v", err)
			}
			const groupSize = 2
			wantCount := (len(urls) + groupSize - 1) / groupSize
			if len(results) != wantCount || len(plan.Operations) != wantCount || calls.Load() != 2 {
				t.Fatalf("results = %d, operations = %d, calls = %d", len(results), len(plan.Operations), calls.Load())
			}
			for i, result := range results {
				wantStatus, wantHTTP := nozzle.NotAttempted, 0
				switch i {
				case 0:
					wantStatus, wantHTTP = nozzle.Accepted, http.StatusOK
				case 1:
					wantStatus, wantHTTP = tt.status, tt.http
				}
				wantURLs := urls[i*groupSize : min((i+1)*groupSize, len(urls))]
				if result.Status != wantStatus || result.HTTPStatus != wantHTTP || !reflect.DeepEqual(result.Operation.URLs, wantURLs) || !reflect.DeepEqual(result.Operation, plan.Operations[i]) {
					t.Fatalf("result %d = %#v, want status %d, HTTP %d, URLs %v", i, result, wantStatus, wantHTTP, wantURLs)
				}
			}
		})
	}
}
