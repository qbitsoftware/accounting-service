package merit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// APIError represents an error response from the Merit API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("merit api: status %d: %s", e.StatusCode, e.Message)
}

// truncate caps s at n bytes for log output, appending an ellipsis marker when
// trimmed so large API bodies don't flood the logs.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// post sends a signed POST request to the Merit API and decodes the JSON response.
// The endpoint should be the path suffix (e.g., "v2/getinvoices").
// The payload is JSON-encoded and included in the signature.
// The result parameter should be a pointer to the expected response type.
func (c *Client) post(ctx context.Context, endpoint string, payload any, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("merit: marshal request: %w", err)
	}

	ts := timestamp()
	sig := sign(c.apiID, c.apiKey, ts, string(body))

	reqURL := fmt.Sprintf("%s%s?ApiId=%s&timestamp=%s&signature=%s",
		c.apiURL, endpoint, c.apiID, ts, urlEncodeSignature(sig))

	slog.Debug("merit api request", "endpoint", endpoint, "body", string(body))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("merit: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("merit: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("merit: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Errors are rare and important — log the (truncated) body at WARN.
		slog.Warn("merit api error", "endpoint", endpoint, "status", resp.StatusCode, "body", truncate(string(respBody), 2000))
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(respBody),
		}
	}

	// Success: log only metadata. The full body floods logs (a single getitems
	// can be hundreds of KB); use Debug level for body inspection when needed.
	slog.Info("merit api response", "endpoint", endpoint, "status", resp.StatusCode, "bytes", len(respBody))
	slog.Debug("merit api response body", "endpoint", endpoint, "body", string(respBody))

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("merit: unmarshal response: %w", err)
		}
	}

	return nil
}
