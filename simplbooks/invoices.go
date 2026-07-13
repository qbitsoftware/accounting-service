package simplbooks

import (
	"context"
	"encoding/json"
	"fmt"
)

// ListInvoices returns invoices matching the given params (single page).
// invoices/list wraps each element under a lowercase "invoices" key.
func (c *Client) ListInvoices(ctx context.Context, params ListParams) ([]Invoice, error) {
	resp, err := c.do(ctx, "GET", "invoices/list", params.body())
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Invoice Invoice `json:"invoices"`
	}
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, fmt.Errorf("simplbooks: parse invoices: %w", err)
	}
	out := make([]Invoice, len(rows))
	for i := range rows {
		out[i] = rows[i].Invoice
	}
	return out, nil
}

// ListAllInvoices pages through invoices/list until exhausted. filter and
// fields are optional; perPage bounds each page.
func (c *Client) ListAllInvoices(ctx context.Context, filter map[string]any, fields []string, perPage int) ([]Invoice, error) {
	if perPage <= 0 {
		perPage = 100
	}
	var all []Invoice
	for page := 1; ; page++ {
		batch, err := c.ListInvoices(ctx, ListParams{Page: page, PerPage: perPage, Fields: fields, Filter: filter})
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < perPage {
			break
		}
	}
	return all, nil
}

// GetInvoice returns a single invoice with its rows.
func (c *Client) GetInvoice(ctx context.Context, id string) (*Invoice, error) {
	resp, err := c.do(ctx, "GET", "invoices/get/"+id, nil)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Invoice Invoice      `json:"Invoice"`
		Task    []InvoiceRow `json:"Task"`
	}
	if err := json.Unmarshal(resp.Data, &payload); err != nil {
		return nil, fmt.Errorf("simplbooks: parse invoice: %w", err)
	}
	inv := payload.Invoice
	inv.Rows = payload.Task
	return &inv, nil
}

// GetInvoicePDF returns the invoice PDF bytes (base64-decoded from the JSON
// envelope's `data` string).
func (c *Client) GetInvoicePDF(ctx context.Context, id string) ([]byte, error) {
	resp, err := c.do(ctx, "GET", "invoices/get_pdf/"+id, nil)
	if err != nil {
		return nil, err
	}
	var b64 string
	if err := json.Unmarshal(resp.Data, &b64); err != nil {
		return nil, fmt.Errorf("simplbooks: parse invoice pdf: %w", err)
	}
	return decodeBase64(b64)
}

// CreateInvoice posts an invoice and returns the new invoice ID. body is the
// full {"Invoice": {...}, "Tasks": [...]} request document.
func (c *Client) CreateInvoice(ctx context.Context, body any) (int, error) {
	resp, err := c.do(ctx, "POST", "invoices/create", body)
	if err != nil {
		return 0, err
	}
	return resp.insertedID()
}

// DeleteInvoice deletes the invoice with the given ID.
func (c *Client) DeleteInvoice(ctx context.Context, id string) error {
	_, err := c.do(ctx, "POST", "invoices/delete/"+id, nil)
	return err
}
