// Package excellentbooks provides a Go client for the Excellent Books (HansaWorld) REST API.
//
// Excellent Books exposes a REST API at /api/1/{Register} with HTTP Basic Auth.
// Registers include IVVc (invoices), CUVc (contacts), INVc (items), VIVc (purchases).
//
// Usage:
//
//	client := excellentbooks.New(excellentbooks.Config{
//	    BaseURL:  "https://test.excellent.ee:3490",
//	    Username: "API",
//	    Password: "secret",
//	})
//
//	customers, err := client.ListCustomers(ctx, excellentbooks.ListParams{Limit: 100})
package excellentbooks

import "net/http"

// Config holds the configuration for an Excellent Books API client.
type Config struct {
	// BaseURL is the server base URL (e.g. "https://test.excellent.ee:3490").
	BaseURL string

	// Username for HTTP Basic Auth.
	Username string

	// Password for HTTP Basic Auth.
	Password string

	// HTTPClient is an optional HTTP client for making requests.
	// Defaults to http.DefaultClient if nil.
	HTTPClient *http.Client
}

// Client is an Excellent Books API client.
type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// New creates a new Excellent Books API client.
func New(cfg Config) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		baseURL:    cfg.BaseURL,
		username:   cfg.Username,
		password:   cfg.Password,
		httpClient: httpClient,
	}
}
