package cloudflare

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestNewPurgeRequestBuildsSingleURLRequest(t *testing.T) {
	const zoneID = "0123456789abcdef0123456789abcdef"
	const token = "test-token-not-a-real-credential"
	const targetURL = "https://example.com/article"

	req, err := newPurgeRequest(t.Context(), zoneID, token, targetURL)
	if err != nil {
		t.Fatal(err)
	}
	if req == nil || req.Body == nil {
		t.Fatal("request or body is nil")
	}
	defer func() {
		if err := req.Body.Close(); err != nil {
			t.Errorf("close request body: %v", err)
		}
	}()

	if req.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", req.Method)
	}
	wantEndpoint := "https://api.cloudflare.com/client/v4/zones/" + zoneID + "/purge_cache"
	if req.URL == nil || req.URL.String() != wantEndpoint {
		t.Errorf("endpoint = %v, want %s", req.URL, wantEndpoint)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer "+token {
		t.Error("bearer authorization does not match the supplied token")
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("content type = %q, want application/json", got)
	}

	var body map[string][]string
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 1 || len(body["files"]) != 1 || body["files"][0] != targetURL {
		t.Errorf("body = %#v, want only files containing the supplied URL", body)
	}
}
