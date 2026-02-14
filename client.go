package accounting

import (
	"context"
	"fmt"
	"net/http"
)

// Config holds configuration for creating a new accounting Client.
type Config struct {
	Provider   string       // Provider name (e.g. "merit")
	APIID      string       // API identifier
	APIKey     string       // API secret key
	Region     string       // Regional endpoint (e.g. "ee", "pl")
	HTTPClient *http.Client // Optional HTTP client; defaults to http.DefaultClient
}

// Client is the main entry point for the accounting SDK.
// Access sub-services via the exported fields.
type Client struct {
	provider  Provider
	Invoices  *InvoiceService
	Customers *CustomerService
	Payments  *PaymentService
	Items     *ItemService
	Purchases *PurchaseService
	Taxes     *TaxService
	Reports   *ReportService
	Sync      *SyncService
}

// NewClient creates a new Client for the configured accounting provider.
func NewClient(cfg Config) (*Client, error) {
	var p Provider
	switch cfg.Provider {
	case "merit":
		p = newMeritProvider(cfg)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, cfg.Provider)
	}

	c := &Client{
		provider:  p,
		Invoices:  &InvoiceService{provider: p},
		Customers: &CustomerService{provider: p},
		Payments:  &PaymentService{provider: p},
		Items:     &ItemService{provider: p},
		Purchases: &PurchaseService{provider: p},
		Taxes:     &TaxService{provider: p},
		Reports:   &ReportService{provider: p},
		Sync:      &SyncService{provider: p},
	}
	return c, nil
}

// TestConnection verifies that the provider credentials are valid.
func (c *Client) TestConnection(ctx context.Context) error {
	return c.provider.TestConnection(ctx)
}
