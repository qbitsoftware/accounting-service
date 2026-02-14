package merit

import "context"

// ListInvoices retrieves a list of sales invoices for the given period.
// The period may not span more than 3 months.
func (c *Client) ListInvoices(ctx context.Context, params ListInvoicesParams) ([]InvoiceListItem, error) {
	var result []InvoiceListItem
	if err := c.post(ctx, "v2/getinvoices", params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetInvoice retrieves detailed information for a single sales invoice.
func (c *Client) GetInvoice(ctx context.Context, params GetInvoiceParams) (*InvoiceDetail, error) {
	var result InvoiceDetail
	if err := c.post(ctx, "v2/getinvoice", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateInvoice creates a new sales invoice.
func (c *Client) CreateInvoice(ctx context.Context, req CreateInvoiceRequest) (*CreateInvoiceResponse, error) {
	var result CreateInvoiceResponse
	if err := c.post(ctx, "v2/sendinvoice", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteInvoice deletes a sales invoice by ID.
func (c *Client) DeleteInvoice(ctx context.Context, params DeleteInvoiceParams) error {
	return c.post(ctx, "v1/deleteinvoice", params, nil)
}
