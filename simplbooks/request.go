package simplbooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strconv"
	"strings"
)

// APIError represents an error response from the SimplBooks API. StatusCode is
// the HTTP status; BodyStatus is the `status` field inside the JSON envelope
// (they usually agree). Messages carries the `errors` array.
type APIError struct {
	StatusCode int
	BodyStatus int
	Messages   []string
}

func (e *APIError) Error() string {
	msg := strings.Join(e.Messages, "; ")
	if msg == "" {
		msg = "request failed"
	}
	return fmt.Sprintf("simplbooks api: status %d: %s", e.StatusCode, msg)
}

// apiResponse is the common envelope returned by every SimplBooks endpoint.
type apiResponse struct {
	Status     int             `json:"status"`
	Duration   json.RawMessage `json:"duration"`
	InsertedID json.RawMessage `json:"inserted_id"` // integer or quoted string ("1")
	Data       json.RawMessage `json:"data"`
	Response   string          `json:"response"`
	Errors     json.RawMessage `json:"errors"` // usually []string
}

// insertedID extracts the new object's ID from a create response. SimplBooks
// returns it either as a JSON number or a quoted string.
func (r *apiResponse) insertedID() (int, error) {
	s := strings.Trim(strings.TrimSpace(string(r.InsertedID)), `"`)
	if s == "" || s == "null" {
		return 0, fmt.Errorf("simplbooks: no inserted_id in response")
	}
	id, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("simplbooks: parse inserted_id %q: %w", s, err)
	}
	if id == 0 {
		return 0, fmt.Errorf("simplbooks: inserted_id is 0")
	}
	return id, nil
}

// parseErrors decodes the `errors` element, which is normally an array of
// strings but may occasionally be a bare string.
func parseErrors(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var single string
	if json.Unmarshal(raw, &single) == nil && single != "" {
		return []string{single}
	}
	return []string{string(raw)}
}

// do performs a request against {baseURL}/{companyID}/api/{path} and returns the
// decoded envelope. `path` is the category/action/id segment (no leading slash),
// e.g. "clients/get/1" or "invoices/create". When body is non-nil it is JSON
// encoded and sent with the X-Input-Format: json header.
func (c *Client) do(ctx context.Context, method, path string, body any) (*apiResponse, error) {
	reqURL := fmt.Sprintf("%s/%s/api/%s", c.baseURL, c.companyID, path)

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("simplbooks: marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	slog.Info("simplbooks request", "method", method, "path", path, "has_body", body != nil)

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("simplbooks: create request: %w", err)
	}
	req.Header.Set("X-Simplbooks-Token", c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("X-Input-Format", "json")
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("simplbooks: send request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("simplbooks: read response: %w", err)
	}

	slog.Info("simplbooks response", "status", resp.StatusCode, "body_len", len(raw))

	var env apiResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		// Non-JSON body — surface HTTP status with the raw payload as message.
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, &APIError{StatusCode: resp.StatusCode, Messages: []string{strings.TrimSpace(string(raw))}}
		}
		return nil, fmt.Errorf("simplbooks: unmarshal response: %w (body: %s)", err, string(raw))
	}

	errs := parseErrors(env.Errors)
	httpOK := resp.StatusCode >= 200 && resp.StatusCode < 300
	bodyOK := env.Status == 0 || (env.Status >= 200 && env.Status < 300)
	if !httpOK || !bodyOK || len(errs) > 0 {
		slog.Error("simplbooks: error response",
			"http_status", resp.StatusCode, "body_status", env.Status, "errors", errs, "raw", string(raw))
		status := resp.StatusCode
		if env.Status != 0 {
			status = env.Status
		}
		return nil, &APIError{StatusCode: status, BodyStatus: env.Status, Messages: errs}
	}

	return &env, nil
}

// ListParams holds the pagination and generic filter fields shared by the
// SimplBooks list endpoints. Filter carries endpoint-specific criteria (e.g.
// {"client_id": 5, "e_mail": "a@b.c"}) merged into the request body. Fields
// requests a specific subset of columns.
type ListParams struct {
	Page    int
	PerPage int
	Fields  []string
	Filter  map[string]any
}

// body builds the JSON request body for a list call, defaulting pagination.
func (p ListParams) body() map[string]any {
	b := map[string]any{}
	maps.Copy(b, p.Filter)
	page := p.Page
	if page <= 0 {
		page = 1
	}
	perPage := p.PerPage
	if perPage <= 0 {
		perPage = 100
	}
	b["page"] = page
	b["per_page"] = perPage
	if len(p.Fields) > 0 {
		b["fields"] = p.Fields
	}
	return b
}
