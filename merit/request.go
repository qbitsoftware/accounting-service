package merit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"
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

// maxRetries bounds how many times a single request is retried after a
// rate-limit or transient upstream failure before giving up.
const maxRetries = 3

// retryableStatus reports whether a response status is worth another attempt.
// 429 is Merit's rate-limit answer; the 5xx trio is a gateway or a restart in
// front of it, which a retry a few seconds later usually clears. Every other
// non-2xx is a real answer about the request itself (a bad period, a duplicate
// invoice number, bad credentials) and retrying it just burns quota.
func retryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// retryBackoff is the delay before retry N (0-based) when the response carries
// no Retry-After. Merit's per-minute window rolls within a minute, so the tail
// waits are long enough to land in a fresh window: 2s, 8s, 20s, 45s.
func retryBackoff(attempt int) time.Duration {
	switch attempt {
	case 0:
		return 2 * time.Second
	case 1:
		return 8 * time.Second
	case 2:
		return 20 * time.Second
	default:
		return 45 * time.Second
	}
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
	if secs, err := strconv.Atoi(v); err == nil {
		if secs <= 0 {
			return 0, false
		}
		return capDelay(time.Duration(secs) * time.Second), true
	}
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d <= 0 {
			return 0, false
		}
		return capDelay(d), true
	}
	return 0, false
}

func capDelay(d time.Duration) time.Duration {
	if d > 60*time.Second {
		return 60 * time.Second
	}
	return d
}

// post sends a signed POST request to the Merit API and decodes the JSON response.
// The endpoint should be the path suffix (e.g., "v2/getinvoices").
// The payload is JSON-encoded and included in the signature.
// The result parameter should be a pointer to the expected response type.
//
// Requests are paced by the client's shared limiter and retried on rate-limit
// and transient upstream failures; see maxRetries.
func (c *Client) post(ctx context.Context, endpoint string, payload any, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("merit: marshal request: %w", err)
	}

	slog.Debug("merit api request", "endpoint", endpoint, "body", string(body))

	var respBody []byte
	var statusCode int

	for attempt := 0; ; attempt++ {
		// Proactive throttle so we stay under Merit's per-API-ID limit. Wait
		// blocks until a token is available or ctx fires. Retries go through
		// here too, so backoff and throttle compose naturally.
		if c.limiter != nil {
			if err := c.limiter.Wait(ctx); err != nil {
				return err
			}
		}

		// Re-sign on every attempt: the signature embeds a timestamp and stale
		// requests are rejected, so a retried request needs a fresh one.
		ts := timestamp()
		sig := sign(c.apiID, c.apiKey, ts, string(body))
		reqURL := fmt.Sprintf("%s%s?ApiId=%s&timestamp=%s&signature=%s",
			c.apiURL, endpoint, c.apiID, ts, urlEncodeSignature(sig))

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("merit: create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("merit: send request: %w", err)
		}
		respBody, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("merit: read response: %w", err)
		}
		statusCode = resp.StatusCode

		if retryableStatus(statusCode) && attempt < maxRetries {
			delay, fromHeader := retryAfter(resp.Header)
			if !fromHeader {
				delay = retryBackoff(attempt)
			}
			slog.Warn("merit api retrying after transient failure",
				"endpoint", endpoint, "status", statusCode, "delay", delay,
				"attempt", attempt+1, "retry_after_header", fromHeader)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			continue
		}
		break
	}

	if statusCode < 200 || statusCode >= 300 {
		// Errors are rare and important — log the (truncated) body at WARN.
		slog.Warn("merit api error", "endpoint", endpoint, "status", statusCode, "body", truncate(string(respBody), 2000))
		return &APIError{
			StatusCode: statusCode,
			Message:    string(respBody),
		}
	}

	// Success: log only metadata. The full body floods logs (a single getitems
	// can be hundreds of KB); use Debug level for body inspection when needed.
	slog.Info("merit api response", "endpoint", endpoint, "status", statusCode, "bytes", len(respBody))
	slog.Debug("merit api response body", "endpoint", endpoint, "body", string(respBody))

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("merit: unmarshal response: %w", err)
		}
	}

	return nil
}
