package smartaccounts

import (
	"context"
	"net/url"
)

// ListVendorInvoicesParams filters a vendorinvoices:get query.
type ListVendorInvoicesParams struct {
	DateFrom      string // dd.MM.yyyy
	DateTo        string // dd.MM.yyyy
	DateType      string // "", "date", "entrydate", "duedate", "modifydate"
	VendorID      string
	InvoiceNumber string
	PaymentStatus string // "", "unpaid", "overdue"
	FetchRows     bool
}

func (p ListVendorInvoicesParams) values() url.Values {
	v := url.Values{}
	setNonEmpty(v, "dateFrom", p.DateFrom)
	setNonEmpty(v, "dateTo", p.DateTo)
	setNonEmpty(v, "dateType", p.DateType)
	setNonEmpty(v, "vendorId", p.VendorID)
	setNonEmpty(v, "invoiceNumber", p.InvoiceNumber)
	setNonEmpty(v, "paymentStatus", p.PaymentStatus)
	if p.FetchRows {
		v.Set("fetchRows", "true")
	}
	return v
}

// ListVendorInvoices retrieves purchase invoices matching params.
func (c *Client) ListVendorInvoices(ctx context.Context, params ListVendorInvoicesParams) ([]VendorInvoiceItem, error) {
	var items []VendorInvoiceItem
	if _, err := c.getList(ctx, "purchasesales/vendorinvoices:get", params.values(), &items); err != nil {
		return nil, err
	}
	return items, nil
}

// GetVendorInvoice fetches a single purchase invoice by ID (rows included).
func (c *Client) GetVendorInvoice(ctx context.Context, id string) (*VendorInvoiceItem, error) {
	v := url.Values{}
	v.Set("id", id)
	v.Set("fetchRows", "true")
	var items []VendorInvoiceItem
	if _, err := c.getList(ctx, "purchasesales/vendorinvoices:get", v, &items); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, &APIError{StatusCode: 404, Message: "vendor invoice not found: " + id}
	}
	return &items[0], nil
}

// CreateVendorInvoice adds a purchase invoice.
func (c *Client) CreateVendorInvoice(ctx context.Context, req CreateVendorInvoiceRequest) (*VendorInvoiceResponse, error) {
	var resp VendorInvoiceResponse
	if err := c.post(ctx, "purchasesales/vendorinvoices:add", nil, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteVendorInvoice deletes a purchase invoice by ID. SA's :delete wants id
// in the URL query AND a non-empty POST body — see DeleteInvoice for details.
func (c *Client) DeleteVendorInvoice(ctx context.Context, id string) error {
	v := url.Values{}
	v.Set("id", id)
	return c.post(ctx, "purchasesales/vendorinvoices:delete", v, struct{}{}, nil)
}
