package merit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// APIError represents an error response from the Merit API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("merit api: status %d: %s", e.StatusCode, e.Message)
}

// do performs a signed POST and returns the raw response body. Shared by
// post (JSON-typed callers) and postRaw (endpoints that return bare text
// like sendinvoiceaseinv).
func (c *Client) do(ctx context.Context, endpoint string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("merit: marshal request: %w", err)
	}

	ts := timestamp()
	sig := sign(c.apiID, c.apiKey, ts, string(body))

	reqURL := fmt.Sprintf("%s%s?ApiId=%s&timestamp=%s&signature=%s",
		c.apiURL, endpoint, c.apiID, ts, urlEncodeSignature(sig))

	slog.Info("merit api request", "endpoint", endpoint, "body", string(body))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("merit: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("merit: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("merit: read response: %w", err)
	}

	slog.Info("merit api response", "endpoint", endpoint, "status", resp.StatusCode, "body", string(respBody))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(respBody),
		}
	}

	return respBody, nil
}

// post sends a signed POST request to the Merit API and decodes the JSON response.
// The endpoint should be the path suffix (e.g., "v2/getinvoices").
// The payload is JSON-encoded and included in the signature.
// The result parameter should be a pointer to the expected response type.
func (c *Client) post(ctx context.Context, endpoint string, payload any, result any) error {
	respBody, err := c.do(ctx, endpoint, payload)
	if err != nil {
		return err
	}
	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("merit: unmarshal response: %w", err)
		}
	}
	return nil
}

// postRaw is used by Merit endpoints that return bare text instead of JSON
// (e.g. sendinvoiceaseinv → "OK" or "api-noeinv"). Strips surrounding quotes
// when Merit happens to JSON-encode the response.
func (c *Client) postRaw(ctx context.Context, endpoint string, payload any) (string, error) {
	respBody, err := c.do(ctx, endpoint, payload)
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(respBody))
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	return s, nil
}
