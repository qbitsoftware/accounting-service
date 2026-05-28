// Package smartaccounts provides a Go client for the SmartAccounts API.
//
// SmartAccounts (https://www.smartaccounts.eu) is an Estonian cloud accounting
// product. Its API is a RESTful-JSON channel where every request carries
// timestamp/apikey/signature query parameters and is signed with HMAC-SHA256
// (hex) using the company's secret key. See auth.go for the signing scheme.
//
// Usage:
//
//	client := smartaccounts.New(smartaccounts.Config{
//	    APIKey:    os.Getenv("SA_API_KEY"),    // public key
//	    SecretKey: os.Getenv("SA_SECRET_KEY"), // private key (HMAC key)
//	})
//
//	invoices, err := client.ListInvoices(ctx, smartaccounts.ListInvoicesParams{
//	    DateFrom: "01.01.2025",
//	})
package smartaccounts

import (
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// DefaultHost is the SmartAccounts API host.
const DefaultHost = "sa.smartaccounts.eu"

// SmartAccounts rate limits per company (apikey): 60 req/min and 1000 req/day.
// We default to a 1 req/s sustained rate with a small burst so short flurries
// (e.g. CreatePayment's lookup + create pair) don't artificially wait, while
// keeping average throughput well under the per-minute cap.
const (
	DefaultRatePerSecond = 1
	DefaultBurst         = 5
)

// Config holds the configuration for a SmartAccounts API client.
type Config struct {
	// Host is the API host. Defaults to DefaultHost if empty.
	Host string

	// Language selects the locale segment of the API path ("en" or "et"),
	// which controls the language of validation messages. Defaults to "en".
	Language string

	// APIKey is the company's public API key, sent as the "apikey" query
	// parameter on every request. Generated in SmartAccounts under
	// Settings >> Connected services.
	APIKey string

	// SecretKey is the company's private/secret key, used as the HMAC-SHA256
	// key when signing requests. It is never transmitted.
	SecretKey string

	// HTTPClient is an optional HTTP client. Defaults to http.DefaultClient.
	HTTPClient *http.Client

	// RatePerSecond and Burst configure proactive client-side throttling so we
	// stay under SmartAccounts' 60 req/min, 1000 req/day per-company limits
	// rather than relying solely on reactive 503/Retry-After backoff. Both
	// zero (the default) → DefaultRatePerSecond / DefaultBurst. Set
	// RatePerSecond to a negative value to disable throttling entirely (tests).
	RatePerSecond int
	Burst         int

	// NettingBank, when set, is the bank account NAME used to post a netting
	// payment that closes a credit invoice against its original immediately on
	// CreateCreditNote. Typically a dedicated offset account (e.g.
	// "Tasaarveldus / Netting" — Estonian accounting practice). Leave empty to
	// skip auto-settling; the credit is still created and linked, but admins
	// settle the balance manually in the SmartAccounts UI.
	NettingBank string
}

// Client is a SmartAccounts API client.
type Client struct {
	baseURL     string // e.g. "https://sa.smartaccounts.eu/en/api/"
	apiKey      string
	secretKey   string
	httpClient  *http.Client
	limiter     *rate.Limiter // nil = no throttling
	nettingBank string        // "" = auto-settle disabled
}

// NettingBank returns the configured netting bank account name, or "" if
// auto-settling of credit notes is disabled.
func (c *Client) NettingBank() string { return c.nettingBank }

// New creates a new SmartAccounts API client with the given configuration.
func New(cfg Config) *Client {
	host := cfg.Host
	if host == "" {
		host = DefaultHost
	}
	lang := cfg.Language
	if lang == "" {
		lang = "en"
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	var limiter *rate.Limiter
	switch {
	case cfg.RatePerSecond < 0:
		limiter = nil // explicitly disabled
	default:
		rps := cfg.RatePerSecond
		if rps == 0 {
			rps = DefaultRatePerSecond
		}
		burst := cfg.Burst
		if burst <= 0 {
			burst = DefaultBurst
		}
		limiter = rate.NewLimiter(rate.Every(time.Second/time.Duration(rps)), burst)
	}

	return &Client{
		baseURL:     "https://" + host + "/" + lang + "/api/",
		apiKey:      cfg.APIKey,
		secretKey:   cfg.SecretKey,
		httpClient:  httpClient,
		limiter:     limiter,
		nettingBank: cfg.NettingBank,
	}
}
