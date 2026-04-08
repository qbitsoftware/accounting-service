package excellentbooks

import (
	"context"
	"encoding/json"
	"fmt"
)

const registerInvoice = "IVVc"

// ListInvoices retrieves sales invoices.
func (c *Client) ListInvoices(ctx context.Context, params ListParams) ([]Invoice, string, error) {
	resp, err := c.get(ctx, registerInvoice, params)
	if err != nil {
		return nil, "", err
	}
	return parseInvoiceResponse(resp)
}

// GetInvoice retrieves a single invoice by serial number.
func (c *Client) GetInvoice(ctx context.Context, serNr string) (*Invoice, error) {
	resp, err := c.getOne(ctx, registerInvoice, serNr)
	if err != nil {
		return nil, err
	}

	invoices, _, err := parseInvoiceResponse(resp)
	if err != nil {
		return nil, err
	}
	if len(invoices) == 0 {
		return nil, &APIError{StatusCode: 404, Message: "invoice not found"}
	}
	return &invoices[0], nil
}

// CreateInvoice creates a new sales invoice. Returns the created invoice.
func (c *Client) CreateInvoice(ctx context.Context, fields map[string]string) (*Invoice, error) {
	resp, err := c.post(ctx, registerInvoice, fields)
	if err != nil {
		return nil, err
	}

	invoices, _, err := parseInvoiceResponse(resp)
	if err != nil {
		return nil, err
	}
	if len(invoices) == 0 {
		return nil, fmt.Errorf("excellentbooks: no invoice returned after create")
	}
	return &invoices[0], nil
}

// UpdateInvoice updates an existing invoice by serial number.
func (c *Client) UpdateInvoice(ctx context.Context, serNr string, fields map[string]string) (*Invoice, error) {
	resp, err := c.patch(ctx, registerInvoice, serNr, fields)
	if err != nil {
		return nil, err
	}

	invoices, _, err := parseInvoiceResponse(resp)
	if err != nil {
		return nil, err
	}
	if len(invoices) == 0 {
		return nil, fmt.Errorf("excellentbooks: no invoice returned after update")
	}
	return &invoices[0], nil
}

// parseInvoiceResponse extracts invoices and sequence from the response.
func parseInvoiceResponse(resp *Response) ([]Invoice, string, error) {
	var envelope struct {
		ResponseMeta
		IVVc []Invoice `json:"IVVc"`
	}
	if err := json.Unmarshal(resp.Data, &envelope); err != nil {
		// Try single record format (when fetching by ID, data wraps differently)
		var singleEnvelope struct {
			ResponseMeta
			IVVc json.RawMessage `json:"IVVc"`
		}
		if err2 := json.Unmarshal(resp.Data, &singleEnvelope); err2 != nil {
			return nil, "", fmt.Errorf("excellentbooks: parse invoices: %w", err)
		}
		// Could be array or single object
		var invoices []Invoice
		if err2 := json.Unmarshal(singleEnvelope.IVVc, &invoices); err2 != nil {
			var inv Invoice
			if err3 := json.Unmarshal(singleEnvelope.IVVc, &inv); err3 != nil {
				return nil, "", fmt.Errorf("excellentbooks: parse invoices: %w", err)
			}
			invoices = []Invoice{inv}
		}
		return invoices, singleEnvelope.Sequence, nil
	}
	return envelope.IVVc, envelope.Sequence, nil
}
