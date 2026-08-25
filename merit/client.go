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

import (
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// Regional API base URLs.
const (
	EstoniaURL = "https://aktiva.merit.ee/api/"
	PolandURL  = "https://program.360ksiegowosc.pl/api/"
)

// Merit allows roughly 100 requests per minute per API ID. Default to 90 with a
// small burst, which leaves headroom for concurrent work on the same
// credentials (a scheduled pull overlapping a manual sync) while short flurries
// — a customer lookup followed by an invoice create — still don't wait.
const (
	DefaultRequestInterval = time.Minute / 90
	DefaultBurst           = 5

	// DefaultTimeout bounds a single request. Merit can be slow but never
	// minutes-slow; without a ceiling a hung connection pins a scheduler worker
	// (and its DB transaction) indefinitely, because http.DefaultClient has no
	// timeout of its own.
	DefaultTimeout = 30 * time.Second
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
	// Defaults to a client with DefaultTimeout if nil.
	HTTPClient *http.Client

	// RequestInterval and Burst configure proactive client-side throttling so
	// we stay under Merit's per-API-ID limit rather than relying solely on
	// reactive 429 backoff. Zero (the default) → DefaultRequestInterval /
	// DefaultBurst.
	RequestInterval time.Duration
	Burst           int

	// DisableRateLimit turns throttling off entirely. Intended for tests
	// against a stub server, where pacing only makes the suite slow.
	DisableRateLimit bool
}

// Client is a Merit Aktiva API client.
type Client struct {
	apiURL     string
	apiID      string
	apiKey     string
	httpClient *http.Client
	limiter    *rate.Limiter // nil = no throttling
}

// New creates a new Merit Aktiva API client with the given configuration.
func New(cfg Config) *Client {
	apiURL := cfg.APIURL
	if apiURL == "" {
		apiURL = EstoniaURL
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}

	var limiter *rate.Limiter
	if !cfg.DisableRateLimit {
		interval := cfg.RequestInterval
		if interval <= 0 {
			interval = DefaultRequestInterval
		}
		burst := cfg.Burst
		if burst <= 0 {
			burst = DefaultBurst
		}
		// Shared per API ID: Merit's quota is per company, and clients are
		// constructed per operation, so a per-instance limiter throttles
		// nothing once two operations for the same company overlap. See
		// limiter.go.
		limiter = sharedLimiter(apiURL, cfg.APIID, interval, burst)
	}

	return &Client{
		apiURL:     apiURL,
		apiID:      cfg.APIID,
		apiKey:     cfg.APIKey,
		httpClient: httpClient,
		limiter:    limiter,
	}
}
