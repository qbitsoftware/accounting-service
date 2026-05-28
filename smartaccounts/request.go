package smartaccounts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

// APIError represents a non-2xx response from the SmartAccounts API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("smartaccounts api: status %d: %s", e.StatusCode, e.Message)
}

// page is the envelope returned by GET methods of change-tracked services.
// The list of objects is delivered under a service-specific key, so callers
// decode into the concrete envelope; this shared shape exposes only the
// pagination/deletion fields that are common to every list response.
type page struct {
	HasMoreEntries bool     `json:"hasMoreEntries"`
	Deleted        []string `json:"deleted"`
}

// do sends a signed request to the SmartAccounts API and decodes the JSON
// response into result (when non-nil).
//
// endpoint is the path suffix after the API base, e.g.
// "purchasesales/clientinvoices:get". params holds caller-supplied query
// parameters; timestamp, apikey and signature are added automatically, with
// signature placed last as required by the signing scheme.
// maxRateLimitRetries bounds how many times a single request is retried after a
// rate-limit response before giving up.
const maxRateLimitRetries = 3

func (c *Client) do(ctx context.Context, method, endpoint string, params url.Values, payload, result any) error {
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload) // compact, no indentation/newlines
		if err != nil {
			return fmt.Errorf("smartaccounts: marshal request: %w", err)
		}
	}

	// Request bodies and responses can carry PII (customer/invoice data), so log
	// them at Debug only; endpoint/status stay at Info for operational tracing.
	slog.Debug("smartaccounts api request", "method", method, "endpoint", endpoint, "body", string(body))

	var respBody []byte
	var statusCode int
	for attempt := 0; ; attempt++ {
		// Proactive throttle so we stay under SmartAccounts' 60/min, 1000/day
		// per-company caps. Wait blocks until a token is available or ctx fires.
		// Retries on rate-limit responses go through here too, so backoff and
		// throttle compose naturally.
		if c.limiter != nil {
			if err := c.limiter.Wait(ctx); err != nil {
				return err
			}
		}
		// Re-sign on every attempt: the signature embeds a timestamp and stale
		// (>15 min) requests are rejected, so a retried request needs a fresh one.
		query := encodeQuery(c.apiKey, params)
		sig := sign(c.secretKey, query+string(body))
		reqURL := c.baseURL + endpoint + "?" + query + "&signature=" + sig

		var reqBody io.Reader
		if body != nil {
			reqBody = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
		if err != nil {
			return fmt.Errorf("smartaccounts: create request: %w", err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json; charset=utf-8")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("smartaccounts: send request: %w", err)
		}
		respBody, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("smartaccounts: read response: %w", err)
		}
		statusCode = resp.StatusCode

		// Retry on rate limiting (SA returns 503 for rate limits and 429 for some
		// proxies). Only retry when a Retry-After is given so we don't hammer a
		// genuine 503 billing/outage, and honour the delay it asks for.
		if (statusCode == http.StatusTooManyRequests || statusCode == http.StatusServiceUnavailable) && attempt < maxRateLimitRetries {
			if delay, ok := retryAfter(resp.Header); ok {
				slog.Warn("smartaccounts api rate-limited; backing off", "endpoint", endpoint, "status", statusCode, "delay", delay, "attempt", attempt+1)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
		}
		break
	}

	slog.Debug("smartaccounts api response", "endpoint", endpoint, "status", statusCode, "body", string(respBody))

	if statusCode < 200 || statusCode >= 300 {
		return &APIError{StatusCode: statusCode, Message: string(respBody)}
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("smartaccounts: unmarshal response: %w", err)
		}
	}
	return nil
}

// retryAfter parses a Retry-After header in either form — delta-seconds
// ("120") or an HTTP date ("Wed, 21 Oct 2015 07:28:00 GMT") — and returns the
// delay to wait, capped at 60s. Returns false when the header is absent, in the
// past, or unparseable.
func retryAfter(h http.Header) (time.Duration, bool) {
	v := h.Get("Retry-After")
	if v == "" {
		return 0, false
	}
	var d time.Duration
	if secs, err := strconv.Atoi(v); err == nil {
		if secs <= 0 {
			return 0, false
		}
		d = time.Duration(secs) * time.Second
	} else if when, perr := http.ParseTime(v); perr == nil {
		d = time.Until(when)
		if d <= 0 {
			return 0, false
		}
	} else {
		return 0, false
	}
	if d > 60*time.Second {
		d = 60 * time.Second
	}
	return d, true
}

// get is a convenience wrapper for a signed GET request with no body.
func (c *Client) get(ctx context.Context, endpoint string, params url.Values, result any) error {
	return c.do(ctx, http.MethodGet, endpoint, params, nil, result)
}

// post is a convenience wrapper for a signed POST request with a JSON body.
func (c *Client) post(ctx context.Context, endpoint string, params url.Values, payload, result any) error {
	return c.do(ctx, http.MethodPost, endpoint, params, payload, result)
}

// getPaginated drives a change-tracked GET service across all its pages,
// invoking collect with each page's raw JSON. It increments pageNumber until
// the response reports hasMoreEntries=false. The deleted IDs reported on the
// first page are returned so callers that care about removals can act on them.
func (c *Client) getPaginated(ctx context.Context, endpoint string, params url.Values, collect func(raw json.RawMessage) error) (deleted []string, err error) {
	if params == nil {
		params = url.Values{}
	}
	// Hard cap so a misbehaving API (one that never sets hasMoreEntries=false or
	// keeps returning the same page) can't spin forever. 1000 pages × 100 rows
	// is far beyond any realistic club's invoice/payment volume per sync window.
	const maxPages = 1000
	for pageNum := 1; pageNum <= maxPages; pageNum++ {
		params.Set("pageNumber", strconv.Itoa(pageNum))

		var raw json.RawMessage
		if err := c.get(ctx, endpoint, params, &raw); err != nil {
			return deleted, err
		}
		if len(raw) == 0 {
			return deleted, nil
		}

		var meta page
		if err := json.Unmarshal(raw, &meta); err != nil {
			return deleted, fmt.Errorf("smartaccounts: unmarshal page meta: %w", err)
		}
		if pageNum == 1 {
			deleted = meta.Deleted
		}

		if err := collect(raw); err != nil {
			return deleted, err
		}
		if !meta.HasMoreEntries {
			return deleted, nil
		}
	}
	return deleted, fmt.Errorf("smartaccounts: pagination exceeded %d pages for %s — aborting (API may not be advancing hasMoreEntries)", maxPages, endpoint)
}

// getList drives a change-tracked GET service across all pages and decodes the
// accumulated items into out (a pointer to a slice). It returns the IDs of
// objects deleted since the queried modifydate (reported on page 1).
//
// The primary data array is located by name-agnostic extraction (see
// extractList): SmartAccounts wraps the list under a service-specific key
// (e.g. "clientInvoices") that the spec does not enumerate, so we take the
// first array in the envelope other than "deleted".
func (c *Client) getList(ctx context.Context, endpoint string, params url.Values, out any) (deleted []string, err error) {
	var all []json.RawMessage
	deleted, err = c.getPaginated(ctx, endpoint, params, func(raw json.RawMessage) error {
		arr, err := extractList(raw)
		if err != nil {
			return err
		}
		var items []json.RawMessage
		if err := json.Unmarshal(arr, &items); err != nil {
			return fmt.Errorf("smartaccounts: unmarshal list items: %w", err)
		}
		all = append(all, items...)
		return nil
	})
	if err != nil {
		return deleted, err
	}
	combined, err := json.Marshal(all)
	if err != nil {
		return deleted, err
	}
	return deleted, json.Unmarshal(combined, out)
}

// getAll fetches a non-paginated reference-data service (the "data for which
// changes cannot be queried" services such as vatpcs/accounts/bankaccounts)
// and decodes its primary array into out.
func (c *Client) getAll(ctx context.Context, endpoint string, params url.Values, out any) error {
	var raw json.RawMessage
	if err := c.get(ctx, endpoint, params, &raw); err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	arr, err := extractList(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(arr, out)
}

// extractList returns the primary data array from a list response. A bare JSON
// array is returned as-is; otherwise the SmartAccounts envelope wraps the list
// under a service-specific key (e.g. "clientInvoices") the spec does not
// enumerate, alongside a "deleted" bookkeeping array.
//
// Because the data key is not documented, we locate the array by shape rather
// than name — but fail loud on ambiguity instead of guessing: exactly one
// non-"deleted" array yields that array; zero arrays yields empty; two or more
// is an error so a future envelope change surfaces here rather than silently
// selecting the wrong list (map iteration would otherwise be non-deterministic).
func extractList(raw json.RawMessage) (json.RawMessage, error) {
	t := bytes.TrimSpace(raw)
	if len(t) > 0 && t[0] == '[' {
		return raw, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("smartaccounts: unmarshal list envelope: %w", err)
	}
	var matchVal json.RawMessage
	candidates := make([]string, 0, 1)
	for k, v := range obj {
		if k == "deleted" {
			continue
		}
		if vt := bytes.TrimSpace(v); len(vt) > 0 && vt[0] == '[' {
			candidates = append(candidates, k)
			matchVal = v
		}
	}
	sort.Strings(candidates)
	switch len(candidates) {
	case 0:
		return json.RawMessage("[]"), nil
	case 1:
		return matchVal, nil
	default:
		return nil, fmt.Errorf("smartaccounts: ambiguous list envelope — multiple array fields %v; cannot determine the data array", candidates)
	}
}

// encodeQuery builds the URL query string for a request: the mandatory
// timestamp and apikey first, then caller params in sorted key order, each
// value URL-encoded. The signature is intentionally not included — it is
// appended by the caller after signing this string.
func encodeQuery(apiKey string, params url.Values) string {
	var b []byte
	b = append(b, "timestamp="...)
	b = append(b, timestamp()...)
	b = append(b, "&apikey="...)
	b = append(b, url.QueryEscape(apiKey)...)

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range params[k] {
			b = append(b, '&')
			b = append(b, url.QueryEscape(k)...)
			b = append(b, '=')
			b = append(b, url.QueryEscape(v)...)
		}
	}
	return string(b)
}
