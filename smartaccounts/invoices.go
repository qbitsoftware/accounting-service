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

// EditInvoice updates an existing client invoice via clientinvoices:edit.
// req.ID must be set to the invoice's SmartAccounts id; the rest of req is the
// full replacement representation (SmartAccounts treats :edit as a full
// overwrite, not a patch), so callers must send every field they want kept —
// including the complete Rows slice.
func (c *Client) EditInvoice(ctx context.Context, req CreateInvoiceRequest) (*InvoiceResponse, error) {
	if req.ID == "" {
		return nil, &APIError{StatusCode: 400, Message: "EditInvoice: req.ID is required"}
	}
	var resp InvoiceResponse
	if err := c.post(ctx, "purchasesales/clientinvoices:edit", nil, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UpdateInvoiceDates changes an invoice's document date and/or due date without
// touching anything else. Because :edit is a full overwrite, it first fetches
// the invoice (rows included) and rebuilds the edit request from it, replacing
// only date/dueDate. Verified against a live tenant to work on both unpaid and
// fully-paid invoices. An empty date or dueDate leaves that field at its current
// value. Dates are "dd.MM.yyyy".
func (c *Client) UpdateInvoiceDates(ctx context.Context, id, date, dueDate string) error {
	inv, err := c.GetInvoice(ctx, id)
	if err != nil {
		return err
	}
	rows := make([]InvoiceRowInput, len(inv.Rows))
	for i, r := range inv.Rows {
		rows[i] = InvoiceRowInput{
			Code:         r.Code,
			Description:  r.Description,
			Price:        r.Price,
			Quantity:     r.Quantity,
			Unit:         r.Unit,
			VatPc:        r.VatPc,
			AccountSales: r.AccountSales,
			ObjectID:     r.ObjectID,
		}
	}
	newDate := date
	if newDate == "" {
		newDate = inv.Date
	}
	newDueDate := dueDate
	if newDueDate == "" {
		newDueDate = inv.DueDate
	}
	req := CreateInvoiceRequest{
		ID:                     inv.ID,
		ClientID:               inv.ClientID,
		Type:                   inv.Type,
		BaseForCreditInvoiceID: inv.BaseForCreditInvoiceID,
		Date:                   newDate,
		DueDate:                newDueDate,
		Currency:               inv.Currency,
		Rows:                   rows,
	}
	_, err = c.EditInvoice(ctx, req)
	return err
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
