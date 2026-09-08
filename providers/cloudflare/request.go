package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

// newPurgeRequest constructs a single-URL purge request without sending it.
func newPurgeRequest(
	ctx context.Context,
	zoneID, apiToken, targetURL string,
) (*http.Request, error) {
	payload := struct {
		Files []string `json:"files"`
	}{
		Files: []string{targetURL},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	endpoint := "https://api.cloudflare.com/client/v4/zones/" +
		zoneID + "/purge_cache"

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}
