package excellentbooks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// dumpFields renders a field map as a stable, sorted "k=v | k=v" string so the
// exact payload sent to EB is visible in logs (compare against the API docs).
func dumpFields(fields map[string]string) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+fields[k])
	}
	return strings.Join(parts, " | ")
}

// extractNestedPATCHError pulls EB error code/field/message out of the
// nested-error PATCH response shape (`{"data":{"messages":[...],"error":{"@code":"...","@field":"..."}}}`).
// Often the body is also truncated (missing closing brace), so we can't
// rely on json.Unmarshal — regex extraction is the only reliable path.
// Returns ("","","") when the body doesn't match the pattern.
func extractNestedPATCHError(body []byte) (code, field, message string) {
	s := string(body)
	if !strings.Contains(s, `"error"`) {
		return "", "", ""
	}
	if m := nestedErrCodeRe.FindStringSubmatch(s); len(m) == 2 {
		code = m[1]
	}
	if m := nestedErrFieldRe.FindStringSubmatch(s); len(m) == 2 {
		field = m[1]
	}
	if m := nestedErrMsgRe.FindStringSubmatch(s); len(m) == 2 {
		message = m[1]
	}
	if code == "" {
		return "", "", ""
	}
	if message == "" && field != "" {
		message = "field " + field + " validation failed (code " + code + ")"
	}
	return code, field, message
}

var (
	nestedErrCodeRe  = regexp.MustCompile(`"@code"\s*:\s*"([^"]*)"`)
	nestedErrFieldRe = regexp.MustCompile(`"@field"\s*:\s*"([^"]*)"`)
	nestedErrMsgRe   = regexp.MustCompile(`"messages"\s*:\s*\[\s*"([^"]*)"`)
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

// encodeForm builds the form body using url.Values.Encode but then converts
// "+" to "%20" for spaces. Standard Books' API parser doesn't decode "+" as
// space (it stores the literal "+" in the field), even though "+" is valid per
// RFC 3986 application/x-www-form-urlencoded. "%20" is universally accepted
// and round-trips correctly. Any literal "+" in user input is already
// percent-encoded as "%2B" by url.Values.Encode, so this replacement is safe.
func encodeForm(fields map[string]string) string {
	form := url.Values{}
	for k, v := range fields {
		form.Set(k, v)
	}
	return strings.ReplaceAll(form.Encode(), "+", "%20")
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
	// PathEscape so codes containing UTF-8 (e.g. "Tõnu-12") or special chars
	// like "+" reach EB intact. Without escaping, bytes go raw into the URL
	// and EB's path parser mishandles them — same family of bug as the
	// charset=UTF-8 fix on form bodies.
	reqURL := fmt.Sprintf("%s/api/%s/%s/%s", c.baseURL, c.companyCode, register, url.PathEscape(id))

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
	// Full payload so the exact set_field/set_row_field format can be compared
	// against the EB API docs when debugging (e.g. credit-note row rejections).
	slog.Info("excellentbooks request payload", "register", register, "form", dumpFields(fields))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(encodeForm(fields)))
	if err != nil {
		return nil, fmt.Errorf("excellentbooks: create request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	// Charset is critical: without it, Standard Books defaults to Latin-1 and
	// percent-decoded UTF-8 bytes get mangled (e.g. "Õ" → "Ã" + control char,
	// causing "code not in use" errors for Estonian-named items / customers).
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("Accept", "application/json")

	return c.doRequest(req)
}

// patch performs a PATCH request with form-encoded body.
func (c *Client) patch(ctx context.Context, register string, id string, fields map[string]string) (*Response, error) {
	// PathEscape — see getOne above for rationale.
	reqURL := fmt.Sprintf("%s/api/%s/%s/%s", c.baseURL, c.companyCode, register, url.PathEscape(id))

	slog.Info("excellentbooks request", "method", "PATCH", "register", register, "id", id)
	slog.Info("excellentbooks request payload", "register", register, "id", id, "form", dumpFields(fields))

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, reqURL, strings.NewReader(encodeForm(fields)))
	if err != nil {
		return nil, fmt.Errorf("excellentbooks: create request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	// Charset is critical: without it, Standard Books defaults to Latin-1 and
	// percent-decoded UTF-8 bytes get mangled (e.g. "Õ" → "Ã" + control char,
	// causing "code not in use" errors for Estonian-named items / customers).
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
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
		// Log the full body so the actual EB error is captured even on non-2xx
		// (the structured error fields are often terse/empty).
		slog.Error("excellentbooks: non-2xx response", "status", resp.StatusCode, "raw_body", string(body))
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
		// EB's PATCH responses occasionally come back as malformed JSON
		// (an unkeyed object literal inside `data`). The write itself has
		// succeeded — status is 2xx and there's no error payload (checked
		// above). Don't fail the operation; downstream callers that try to
		// parse the body will get empty data and can handle it.
		if req.Method == http.MethodPatch {
			// Before giving up and treating as success, scan the raw body
			// for the nested-error shape EB sometimes returns on PATCH
			// rejections: `{"data":{"messages":[...],"error":{"@code":"1256","@field":"PayDeal"}}}`
			// (often truncated, so a strict JSON unmarshal fails — but the
			// substring is still parseable enough to extract the code +
			// field with regex). Without this branch, real EB validation
			// errors get logged as "treating as success" and callers have
			// no idea their write was rejected.
			if errCode, errField, errMsg := extractNestedPATCHError(body); errCode != "" {
				slog.Warn("excellentbooks: PATCH rejected with nested-error payload",
					"method", req.Method,
					"url", req.URL.String(),
					"error_code", errCode,
					"error_field", errField,
					"message", errMsg)
				return nil, &APIError{
					StatusCode: resp.StatusCode,
					Message:    errMsg,
					ErrorCode:  errCode,
					ErrorField: errField,
				}
			}
			// Log the raw body so we can diagnose whether EB actually
			// rejected the change (silently) vs succeeded with a truncated
			// response. Bounded to first 500 bytes to keep log lines sane.
			rawSnippet := string(body)
			if len(rawSnippet) > 500 {
				rawSnippet = rawSnippet[:500] + "...(truncated)"
			}
			slog.Warn("excellentbooks: PATCH succeeded but response body was malformed; treating as success",
				"method", req.Method,
				"url", req.URL.String(),
				"body_len", len(body),
				"unmarshal_error", err,
				"raw_body", rawSnippet)
			return &Response{Data: []byte("{}")}, nil
		}
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
