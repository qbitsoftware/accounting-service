package simplbooks

import (
	"context"
	"encoding/json"
	"fmt"
)

// ListIncomings returns customer receipts (money in) matching the params.
func (c *Client) ListIncomings(ctx context.Context, params ListParams) ([]Incoming, error) {
	resp, err := c.do(ctx, "GET", "incomings/list", params.body())
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Incoming Incoming `json:"Incoming"`
	}
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, fmt.Errorf("simplbooks: parse incomings: %w", err)
	}
	out := make([]Incoming, len(rows))
	for i := range rows {
		out[i] = rows[i].Incoming
	}
	return out, nil
}

// ListAllIncomings pages through incomings/list until exhausted.
func (c *Client) ListAllIncomings(ctx context.Context, filter map[string]any, perPage int) ([]Incoming, error) {
	if perPage <= 0 {
		perPage = 100
	}
	var all []Incoming
	for page := 1; ; page++ {
		batch, err := c.ListIncomings(ctx, ListParams{Page: page, PerPage: perPage, Filter: filter})
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

// CreateIncoming records a customer receipt. When invoiceID is non-zero the
// receipt is bound to that invoice (settling it if the sum covers the balance).
func (c *Client) CreateIncoming(ctx context.Context, fields map[string]any, invoiceID int) (int, error) {
	body := map[string]any{"Incoming": fields}
	if invoiceID != 0 {
		body["invoice_id"] = invoiceID
	}
	resp, err := c.do(ctx, "POST", "incomings/create", body)
	if err != nil {
		return 0, err
	}
	return resp.insertedID()
}

// BindIncomingToInvoice ties an existing receipt to an invoice, marking the
// invoice paid when the receipt sum is large enough.
func (c *Client) BindIncomingToInvoice(ctx context.Context, incomingID, invoiceID int, fullCover bool) error {
	_, err := c.do(ctx, "POST", "incomings/invoice_paid", map[string]any{
		"incoming_id": incomingID,
		"invoice_id":  invoiceID,
		"full_cover":  fullCover,
	})
	return err
}

// DeleteIncoming deletes a customer receipt.
func (c *Client) DeleteIncoming(ctx context.Context, id string) error {
	_, err := c.do(ctx, "POST", "incomings/delete/"+id, nil)
	return err
}
