package excellentbooks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// APIError represents an error response from the Excellent Books API.
type APIError struct {
	StatusCode int
	Message    string
	ErrorCode  string
	ErrorField string
}

func (e *APIError) Error() string {
	if e.ErrorField != "" {
		return fmt.Sprintf("excellentbooks api: status %d: field %s: %s", e.StatusCode, e.ErrorField, e.Message)
	}
	return fmt.Sprintf("excellentbooks api: status %d: %s", e.StatusCode, e.Message)
}

// ListParams specifies common query parameters for GET requests.
type ListParams struct {
	Fields       string            // Comma-separated field list
	Limit        int
	Offset       int
	Sort         string
	Range        string
	UpdatesAfter string            // Sequence value for incremental sync
	Filter       map[string]string // Exact-match filters: {"RefStr": "ORD-001"}
}

func (p ListParams) toValues() url.Values {
	v := url.Values{}
	if p.Fields != "" {
		v.Set("fields", p.Fields)
	}
	if p.Limit > 0 {
		v.Set("limit", fmt.Sprintf("%d", p.Limit))
	}
	if p.Offset > 0 {
		v.Set("offset", fmt.Sprintf("%d", p.Offset))
	}
	if p.Sort != "" {
		v.Set("sort", p.Sort)
	}
	if p.Range != "" {
		v.Set("range", p.Range)
	}
	if p.UpdatesAfter != "" {
		v.Set("updates_after", p.UpdatesAfter)
	}
	for field, val := range p.Filter {
		v.Set("filter."+field, val)
	}
	return v
}

// get performs a GET request and decodes the JSON response.
func (c *Client) get(ctx context.Context, register string, params ListParams) (*Response, error) {
	reqURL := fmt.Sprintf("%s/api/%s/%s", c.baseURL, c.companyCode, register)
	qp := params.toValues()
	if len(qp) > 0 {
		reqURL += "?" + qp.Encode()
	}

	slog.Info("excellentbooks request", "method", "GET", "register", register, "url", reqURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("excellentbooks: create request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")

	return c.doRequest(req)
}

// getOne performs a GET request for a single record by ID.
func (c *Client) getOne(ctx context.Context, register string, id string) (*Response, error) {
	reqURL := fmt.Sprintf("%s/api/%s/%s/%s", c.baseURL, c.companyCode, register, id)

	slog.Info("excellentbooks request", "method", "GET", "register", register, "id", id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("excellentbooks: create request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")

	return c.doRequest(req)
}

// post performs a POST request with form-encoded body.
func (c *Client) post(ctx context.Context, register string, fields map[string]string) (*Response, error) {
	reqURL := fmt.Sprintf("%s/api/%s/%s", c.baseURL, c.companyCode, register)

	slog.Info("excellentbooks request", "method", "POST", "register", register, "fields", len(fields))

	form := url.Values{}
	for k, v := range fields {
		form.Set(k, v)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("excellentbooks: create request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	return c.doRequest(req)
}

// patch performs a PATCH request with form-encoded body.
func (c *Client) patch(ctx context.Context, register string, id string, fields map[string]string) (*Response, error) {
	reqURL := fmt.Sprintf("%s/api/%s/%s/%s", c.baseURL, c.companyCode, register, id)

	slog.Info("excellentbooks request", "method", "PATCH", "register", register, "id", id)

	form := url.Values{}
	for k, v := range fields {
		form.Set(k, v)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("excellentbooks: create request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	return c.doRequest(req)
}

// doRequest executes the HTTP request and parses the response.
func (c *Client) doRequest(req *http.Request) (*Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("excellentbooks: send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("excellentbooks: read response: %w", err)
	}

	slog.Info("excellentbooks response", "status", resp.StatusCode, "body_len", len(body))

	// Try to parse error response
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp errorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Code != "" {
			return nil, &APIError{
				StatusCode: resp.StatusCode,
				Message:    errResp.Error.Description,
				ErrorCode:  errResp.Error.Code,
				ErrorField: errResp.Error.Field,
			}
		}
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
		}
	}

	// EB sometimes returns 200 with an error payload — check before treating as success
	var errCheck errorResponse
	if json.Unmarshal(body, &errCheck) == nil && errCheck.Error.Code != "" {
		// Log the full body so we can see what EB really said. The structured
		// description field is often terse / cryptic — the raw body usually
		// contains the actual problem.
		slog.Error("excellentbooks: API returned error payload",
			"status", resp.StatusCode,
			"error_code", errCheck.Error.Code,
			"error_field", errCheck.Error.Field,
			"error_description", errCheck.Error.Description,
			"messages", errCheck.Messages,
			"raw_body", string(body))
		message := errCheck.Error.Description
		if message == "" && len(errCheck.Messages) > 0 {
			message = strings.Join(errCheck.Messages, "; ")
		}
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    message,
			ErrorCode:  errCheck.Error.Code,
			ErrorField: errCheck.Error.Field,
		}
	}

	var result Response
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("excellentbooks: unmarshal response: %w (body: %s)", err, string(body))
	}

	return &result, nil
}

// errorResponse is the JSON error format from Excellent Books.
type errorResponse struct {
	Messages []string `json:"messages"`
	Error    struct {
		Code        string `json:"@code"`
		Description string `json:"@description"`
		Field       string `json:"@field"`
	} `json:"error"`
}
