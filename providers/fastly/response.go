package fastly

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-cdnkit/nozzle"
)

// Errors omit remote response text to avoid exposing sensitive data.
func parsePurgeResponse(body []byte, httpStatus int) (nozzle.Status, error) {
	invalid := errors.New("invalid purge response")
	decoder := json.NewDecoder(bytes.NewReader(body))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nozzle.Indeterminate, invalid
	}
	var status *string
	var message, detail string
	var responseErrors []json.RawMessage
	seen := make(map[string]bool, 4)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nozzle.Indeterminate, invalid
		}
		key, ok := token.(string)
		if !ok {
			return nozzle.Indeterminate, invalid
		}
		switch key {
		case "status", "msg", "detail", "errors":
			if seen[key] {
				return nozzle.Indeterminate, invalid
			}
			seen[key] = true
		}
		switch key {
		case "status":
			if err := decoder.Decode(&status); err != nil || status == nil {
				return nozzle.Indeterminate, invalid
			}
		case "msg":
			if err := decoder.Decode(&message); err != nil {
				return nozzle.Indeterminate, invalid
			}
		case "detail":
			if err := decoder.Decode(&detail); err != nil {
				return nozzle.Indeterminate, invalid
			}
		case "errors":
			if err := decoder.Decode(&responseErrors); err != nil {
				return nozzle.Indeterminate, invalid
			}
		default:
			var ignored json.RawMessage
			if err := decoder.Decode(&ignored); err != nil {
				return nozzle.Indeterminate, invalid
			}
		}
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nozzle.Indeterminate, invalid
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nozzle.Indeterminate, invalid
	}
	if httpStatus >= 200 && httpStatus < 300 && status != nil && *status == "ok" && message == "" && detail == "" && len(responseErrors) == 0 {
		return nozzle.Accepted, nil
	}
	if httpStatus >= 400 && httpStatus < 500 && httpStatus != http.StatusRequestTimeout && status == nil && message != "" && len(responseErrors) == 0 {
		return nozzle.Rejected, errors.New("purge operation was rejected")
	}
	return nozzle.Indeterminate, errors.New("purge acceptance was not confirmed")
}
