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

// SmartAccounts processes at most 60 requests per minute and 1000 requests per
// 24 hours, per company; anything beyond either limit is answered 503 ("Rate
// Limit Exceeded"). Both figures are from SmartAccounts_API_latest.pdf,
// § Request limits — note 503 is also their billing-error code, so a 503 is not
// proof of rate limiting on its own.
//
// A sustained 1 req/s sits exactly ON the per-minute cap, so any burst — or any
// other call sharing the same company key — crosses it and SA answers 503 for
// every subsequent request until the window rolls. Default to ~45 req/min with a
// small burst, which leaves headroom for concurrent work on the same key while
// short flurries (e.g. CreatePayment's lookup + create pair) still don't wait.
//
// The 24h limit is NOT enforced here: 1000/day is a budget question for the
// caller (how often it syncs, how many entities it sweeps), not something a
// per-request throttle can smooth away.
const (
	DefaultRequestInterval = 1333 * time.Millisecond // ~45 req/min
	DefaultBurst           = 3

	// DefaultRatePerSecond is the legacy whole-requests-per-second default.
	// Deprecated: superseded by DefaultRequestInterval, which can express a
	// sub-1/s rate. Kept so existing callers still compile.
	DefaultRatePerSecond = 1
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
	// zero (the default) → DefaultRequestInterval / DefaultBurst. Set
	// RatePerSecond to a negative value to disable throttling entirely (tests).
	// RequestInterval expresses the same thing with sub-second granularity and
	// wins over RatePerSecond when both are set.
	RatePerSecond   int
	RequestInterval time.Duration
	Burst           int

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
	budget      *dayBudget    // 24h request meter, shared per company; nil = not metered
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
		interval := cfg.RequestInterval
		if interval <= 0 && cfg.RatePerSecond > 0 {
			interval = time.Second / time.Duration(cfg.RatePerSecond)
		}
		if interval <= 0 {
			interval = DefaultRequestInterval
		}
		burst := cfg.Burst
		if burst <= 0 {
			burst = DefaultBurst
		}
		// Shared per API key: SmartAccounts' quota is per company, and clients are
		// constructed per operation, so a per-instance limiter throttles nothing
		// once two operations for the same company overlap. See limiter.go.
		limiter = sharedLimiter(host, cfg.APIKey, interval, burst)
	}

	return &Client{
		baseURL:    "https://" + host + "/" + lang + "/api/",
		apiKey:     cfg.APIKey,
		secretKey:  cfg.SecretKey,
		httpClient: httpClient,
		limiter:    limiter,
		// Shared per company for the same reason the limiter is: clients are
		// built per operation, so a meter living on one of them would report a
		// fraction of the company's spend as the whole of it.
		budget:      sharedBudget(host, cfg.APIKey),
		nettingBank: cfg.NettingBank,
	}
}
