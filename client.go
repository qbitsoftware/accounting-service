package accounting

import (
	"context"
	"fmt"
	"net/http"
)

// Config holds configuration for creating a new accounting Client.
type Config struct {
	Provider   string            // Provider name (e.g. "merit", "directo", "excellentbooks")
	APIID      string            // API identifier (Merit: API ID, Directo: company code)
	APIKey     string            // API secret key (Merit: API key, Directo: XML token)
	Region     string            // Regional endpoint (e.g. "ee", "pl")
	HTTPClient *http.Client      // Optional HTTP client; defaults to http.DefaultClient
	Extra      map[string]string // Provider-specific config (e.g. "rest_api_key" for Directo)
}

// Client is the main entry point for the accounting SDK.
// Access sub-services via the exported fields.
type Client struct {
	provider     Provider
	providerName string
	Invoices     *InvoiceService
	Customers    *CustomerService
	Payments     *PaymentService
	Items        *ItemService
	Purchases    *PurchaseService
	Taxes        *TaxService
	Reports      *ReportService
	Sync         *SyncService
}

// NewClient creates a new Client for the configured accounting provider.
func NewClient(cfg Config) (*Client, error) {
	var p Provider
	var err error
	switch cfg.Provider {
	case "merit":
		p = newMeritProvider(cfg)
	case "directo":
		p, err = newDirectoProvider(cfg)
		if err != nil {
			return nil, err
		}
	case "excellentbooks":
		p = newExcellentProvider(cfg)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, cfg.Provider)
	}

	c := &Client{
		provider:     p,
		providerName: cfg.Provider,
		Invoices:     &InvoiceService{provider: p},
		Customers:    &CustomerService{provider: p},
		Payments:     &PaymentService{provider: p},
		Items:        &ItemService{provider: p},
		Purchases:    &PurchaseService{provider: p},
		Taxes:        &TaxService{provider: p},
		Reports:      &ReportService{provider: p},
		Sync:         &SyncService{provider: p},
	}
	return c, nil
}

// TestConnection verifies that the provider credentials are valid.
func (c *Client) TestConnection(ctx context.Context) error {
	return c.provider.TestConnection(ctx)
}

// Capabilities returns the feature set supported by the configured provider.
func (c *Client) Capabilities() Capabilities {
	return ProviderCapabilities(c.providerName)
}
