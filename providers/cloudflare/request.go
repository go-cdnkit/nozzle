package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

func newPurgeRequest(
	ctx context.Context,
	zoneID, apiToken, targetURL string,
) (*http.Request, error) {
	return newBatchPurgeRequest(ctx, zoneID, apiToken, []string{targetURL})
}

func newBatchPurgeRequest(ctx context.Context, zoneID, apiToken string, urls []string) (*http.Request, error) {
	body, err := json.Marshal(struct {
		Files []string `json:"files"`
	}{Files: urls})
	if err != nil {
		return nil, err
	}
	endpoint := "https://api.cloudflare.com/client/v4/zones/" + zoneID + "/purge_cache"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}
