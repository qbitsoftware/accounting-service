package directo

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// xmlClient handles read+write operations via the Directo XML Direct API.
type xmlClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// XMLResult represents a single result entry from the XML Direct API response.
type XMLResult struct {
	XMLName xml.Name `xml:"result"`
	What    string   `xml:"what,attr"`
	Type    string   `xml:"type,attr"`
	Desc    string   `xml:"desc,attr"`
	Code    string   `xml:"code,attr"`
	Status  string   `xml:"status,attr"`
	Error   string   `xml:"error,attr"`
	Msg     string   `xml:"msg,attr"`
}

// XMLResults wraps multiple results from an XML Direct response.
type XMLResults struct {
	XMLName xml.Name    `xml:"results"`
	Results []XMLResult `xml:"result"`
}

// xmlPut performs a PUT (write) operation via XML Direct.
func (c *xmlClient) xmlPut(ctx context.Context, what string, xmlData string, extraParams url.Values) (*XMLResults, error) {
	params := url.Values{
		"put":     {"1"},
		"what":    {what},
		"TOKEN":   {c.token},
		"KEY":     {c.token},
		"xmldata": {xmlData},
	}
	for k, vs := range extraParams {
		for _, v := range vs {
			params.Add(k, v)
		}
	}

	slog.Info("directo xml put", "what", what, "xmldata_len", len(xmlData))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("directo xml: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("directo xml: send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("directo xml: read response: %w", err)
	}

	slog.Info("directo xml put response", "what", what, "status", resp.StatusCode, "body", string(body))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
			Source:     "xml",
		}
	}

	// Check for token error
	bodyStr := string(body)
	if strings.Contains(bodyStr, "token required") || strings.Contains(bodyStr, "Unauthorized") {
		return nil, &APIError{
			StatusCode: 401,
			Message:    bodyStr,
			Source:     "xml",
		}
	}

	var results XMLResults
	if err := xml.Unmarshal(body, &results); err != nil {
		// Some responses may be a single result, not wrapped in <results>
		var single XMLResult
		if err2 := xml.Unmarshal(body, &single); err2 == nil {
			results.Results = []XMLResult{single}
		} else {
			return nil, fmt.Errorf("directo xml: unmarshal response: %w (body: %s)", err, bodyStr)
		}
	}

	// Check for error results
	// Type 0 = success, Type 1 = failure (duplicate, validation), Type 5 = unauthorized
	for _, r := range results.Results {
		if r.Type == "5" {
			return nil, &APIError{
				StatusCode: 401,
				Message:    r.Desc,
				Source:     "xml",
			}
		}
		if r.Type == "1" {
			msg := r.Desc
			if msg == "" {
				msg = r.Msg
			}
			if msg == "" {
				msg = "operation failed"
			}
			return nil, &APIError{
				StatusCode: 400,
				Message:    msg,
				Source:     "xml",
			}
		}
		if r.Error != "" {
			return nil, &APIError{
				StatusCode: 400,
				Message:    r.Error,
				Source:     "xml",
			}
		}
	}

	return &results, nil
}

// xmlGet performs a GET (read) operation via XML Direct.
func (c *xmlClient) xmlGet(ctx context.Context, what string, extraParams url.Values) ([]byte, error) {
	params := url.Values{
		"get":   {"1"},
		"what":  {what},
		"TOKEN": {c.token},
		"KEY":   {c.token},
	}
	for k, vs := range extraParams {
		for _, v := range vs {
			params.Add(k, v)
		}
	}

	reqURL := c.baseURL + "?" + params.Encode()

	slog.Info("directo xml get", "what", what, "url", reqURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("directo xml: create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("directo xml: send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("directo xml: read response: %w", err)
	}

	slog.Info("directo xml get response", "what", what, "status", resp.StatusCode, "body_len", len(body))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
			Source:     "xml",
		}
	}

	bodyStr := string(body)
	if strings.Contains(bodyStr, "token required") || strings.Contains(bodyStr, "Unauthorized") {
		return nil, &APIError{
			StatusCode: 401,
			Message:    bodyStr,
			Source:     "xml",
		}
	}

	return body, nil
}
