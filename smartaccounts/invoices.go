package smartaccounts

import (
	"context"
	"fmt"
	"net/url"
)

// ListInvoicesParams filters a clientinvoices:get query.
type ListInvoicesParams struct {
	DateFrom      string // dd.MM.yyyy
	DateTo        string // dd.MM.yyyy
	DateType      string // "", "date", "entrydate", "duedate", "modifydate"
	ClientID      string
	InvoiceNumber string
	PaymentStatus string // "", "unpaid", "overdue"
	FetchRows     bool
}

func (p ListInvoicesParams) values() url.Values {
	v := url.Values{}
	setNonEmpty(v, "dateFrom", p.DateFrom)
	setNonEmpty(v, "dateTo", p.DateTo)
	setNonEmpty(v, "dateType", p.DateType)
	setNonEmpty(v, "clientId", p.ClientID)
	setNonEmpty(v, "invoiceNumber", p.InvoiceNumber)
	setNonEmpty(v, "paymentStatus", p.PaymentStatus)
	if p.FetchRows {
		v.Set("fetchRows", "true")
	}
	return v
}

// ListInvoices retrieves client invoices matching params, following pagination.
// The second return value holds IDs deleted since the queried modifydate.
func (c *Client) ListInvoices(ctx context.Context, params ListInvoicesParams) ([]InvoiceItem, []string, error) {
	var items []InvoiceItem
	deleted, err := c.getList(ctx, "purchasesales/clientinvoices:get", params.values(), &items)
	if err != nil {
		return nil, nil, err
	}
	return items, deleted, nil
}

// GetInvoice fetches a single client invoice by ID (rows included).
func (c *Client) GetInvoice(ctx context.Context, id string) (*InvoiceItem, error) {
	v := url.Values{}
	v.Set("id", id)
	v.Set("fetchRows", "true")
	var items []InvoiceItem
	if _, err := c.getList(ctx, "purchasesales/clientinvoices:get", v, &items); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, &APIError{StatusCode: 404, Message: "invoice not found: " + id}
	}
	return &items[0], nil
}

// FindInvoiceByNumber fetches the single client invoice with the given invoice
// number. It errors if no invoice matches, or if more than one does — Estonian
// numbering can repeat across years, and silently taking the first match would
// attribute a payment/credit to the wrong document.
func (c *Client) FindInvoiceByNumber(ctx context.Context, invoiceNumber string) (*InvoiceItem, error) {
	// SmartAccounts rejects clientinvoices:get queries where none of
	// {dateFrom, paymentStatus, id} is supplied ("dateFrom only allowed to be
	// null when paymentStatus or id is provided"), so invoiceNumber alone is
	// not enough. Bound the search with a wide dateFrom that comfortably
	// covers any realistic invoice age while keeping pagination tight when
	// invoiceNumber matches uniquely.
	items, _, err := c.ListInvoices(ctx, ListInvoicesParams{
		InvoiceNumber: invoiceNumber,
		DateFrom:      "01.01.2000",
		FetchRows:     true,
	})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, &APIError{StatusCode: 404, Message: "invoice not found: " + invoiceNumber}
	}
	if len(items) > 1 {
		return nil, &APIError{StatusCode: 409, Message: fmt.Sprintf("ambiguous invoice number %q: %d matches", invoiceNumber, len(items))}
	}
	return &items[0], nil
}

// CreateInvoice adds a client invoice.
func (c *Client) CreateInvoice(ctx context.Context, req CreateInvoiceRequest) (*InvoiceResponse, error) {
	var resp InvoiceResponse
	if err := c.post(ctx, "purchasesales/clientinvoices:add", nil, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteInvoice deletes a client invoice by ID. SmartAccounts' :delete methods
// want the id in the URL query (like :get) AND a non-empty POST body — sending
// {"id":...} in the body alone yields "Invoice by id not found", sending no
// body yields "POST body read error". We pass id via the query and `{}` as the
// body to satisfy both gates.
func (c *Client) DeleteInvoice(ctx context.Context, id string) error {
	v := url.Values{}
	v.Set("id", id)
	return c.post(ctx, "purchasesales/clientinvoices:delete", v, struct{}{}, nil)
}

// GetInvoicePDF renders the PDF for a client invoice.
func (c *Client) GetInvoicePDF(ctx context.Context, id string) (*PDFResponse, error) {
	v := url.Values{}
	v.Set("id", id)
	var resp PDFResponse
	if err := c.get(ctx, "purchasesales/clientinvoices:getpdf", v, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// setNonEmpty adds key=val to v only when val is non-empty.
func setNonEmpty(v url.Values, key, val string) {
	if val != "" {
		v.Set(key, val)
	}
}
