package directo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
)

// restClient handles read operations via the Directo REST API.
type restClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// get performs a GET request to the REST API.
func (c *restClient) get(ctx context.Context, endpoint string, params url.Values, result any) error {
	reqURL := c.baseURL + endpoint
	if params != nil && len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	slog.Info("directo rest request", "endpoint", endpoint, "url", reqURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("directo rest: create request: %w", err)
	}
	req.Header.Set("X-Directo-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("directo rest: send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("directo rest: read response: %w", err)
	}

	slog.Info("directo rest response", "endpoint", endpoint, "status", resp.StatusCode, "body_len", len(body))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
			Source:     "rest",
		}
	}

	if result != nil && len(body) > 0 {
		if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("directo rest: unmarshal response: %w (body: %s)", err, string(body))
		}
	}

	return nil
}
