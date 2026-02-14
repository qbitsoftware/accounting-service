// Package merit provides a Go client for the Merit Aktiva accounting API.
//
// Merit Aktiva is a cloud accounting solution used in Estonia, Finland, and Poland.
// All API endpoints use HTTP POST with HMAC-SHA256 authentication.
//
// Usage:
//
//	client := merit.New(merit.Config{
//	    APIID:  os.Getenv("MERIT_API_ID"),
//	    APIKey: os.Getenv("MERIT_API_KEY"),
//	})
//
//	invoices, err := client.ListInvoices(ctx, merit.ListInvoicesParams{
//	    PeriodStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
//	    PeriodEnd:   time.Now(),
//	})
package merit

import "net/http"

// Regional API base URLs.
const (
	EstoniaURL = "https://aktiva.merit.ee/api/"
	PolandURL  = "https://program.360ksiegowosc.pl/api/"
)

// Config holds the configuration for a Merit Aktiva API client.
type Config struct {
	// APIURL is the base URL for the Merit API.
	// Defaults to EstoniaURL if empty.
	APIURL string

	// APIID is the API identifier generated in Merit Settings >> API.
	APIID string

	// APIKey is the API secret key generated in Merit Settings >> API.
	APIKey string

	// HTTPClient is an optional HTTP client for making requests.
	// Defaults to http.DefaultClient if nil.
	HTTPClient *http.Client
}

// Client is a Merit Aktiva API client.
type Client struct {
	apiURL     string
	apiID      string
	apiKey     string
	httpClient *http.Client
}

// New creates a new Merit Aktiva API client with the given configuration.
func New(cfg Config) *Client {
	apiURL := cfg.APIURL
	if apiURL == "" {
		apiURL = EstoniaURL
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		apiURL:     apiURL,
		apiID:      cfg.APIID,
		apiKey:     cfg.APIKey,
		httpClient: httpClient,
	}
}
