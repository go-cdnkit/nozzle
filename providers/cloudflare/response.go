package cloudflare

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// Errors omit remote response text to avoid exposing sensitive data.
func parsePurgeResponse(body []byte) (bool, error) {
	invalid := errors.New("invalid purge response")
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return false, invalid
	}
	var success *bool
	var responseErrors []json.RawMessage
	var seenSuccess, seenErrors bool
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return false, invalid
		}
		switch key {
		case "success":
			if seenSuccess {
				return false, invalid
			}
			seenSuccess = true
			if err := decoder.Decode(&success); err != nil {
				return false, invalid
			}
		case "errors":
			if seenErrors {
				return false, invalid
			}
			seenErrors = true
			if err := decoder.Decode(&responseErrors); err != nil {
				return false, invalid
			}
		default:
			var ignored json.RawMessage
			if err := decoder.Decode(&ignored); err != nil {
				return false, invalid
			}
		}
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return false, invalid
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return false, invalid
	}
	if success == nil || *success && len(responseErrors) != 0 {
		return false, invalid
	}
	return *success, nil
}
