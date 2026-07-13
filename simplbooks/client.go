// Package simplbooks provides a Go client for the SimplBooks accounting API.
//
// SimplBooks exposes a REST-ish API at
//
//	https://app.simplbooks.com/{company_id}/api/{category}/{action}/{id}
//
// authenticated with a per-account token in the X-Simplbooks-Token header.
// Requests and responses are JSON (X-Input-Format / X-Output-Format headers
// select the format; JSON is the default here). Every response is wrapped in a
// common envelope: {status, duration, inserted_id, data, response, errors}.
//
// The API is Premium-plan-only and rate limited to 60 requests/minute.
//
// Usage:
//
//	client := simplbooks.New(simplbooks.Config{
//	    CompanyID: "0fdf33a5c10c384986f81fc45a444f22",
//	    Token:     "5e7c2b8035bb7533453450f7bce3e3cd",
//	})
//	clients, err := client.ListClients(ctx, simplbooks.ListParams{})
package simplbooks

import (
	"net/http"
	"strings"
)

// DefaultBaseURL is the SimplBooks production host.
const DefaultBaseURL = "https://app.simplbooks.com"

// Config holds the configuration for a SimplBooks API client.
type Config struct {
	// BaseURL is the API host. Defaults to DefaultBaseURL if empty.
	BaseURL string

	// CompanyID is the unique account identifier that forms the first path
	// segment of every request URL (e.g. "0fdf33a5c10c384986f81fc45a444f22").
	CompanyID string

	// Token is the API user's token, sent in the X-Simplbooks-Token header.
	Token string

	// HTTPClient is an optional HTTP client. Defaults to http.DefaultClient.
	HTTPClient *http.Client
}

// Client is a SimplBooks API client.
type Client struct {
	baseURL    string
	companyID  string
	token      string
	httpClient *http.Client
}

// New creates a new SimplBooks API client.
func New(cfg Config) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL:    baseURL,
		companyID:  cfg.CompanyID,
		token:      cfg.Token,
		httpClient: httpClient,
	}
}
