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

// GetInvoicePDF retrieves the PDF for a sales invoice.
func (c *Client) GetInvoicePDF(ctx context.Context, params GetInvoicePDFParams) (*Attachment, error) {
	var result Attachment
	if err := c.post(ctx, "v2/getsalesinvpdf", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteInvoice deletes a sales invoice by ID.
func (c *Client) DeleteInvoice(ctx context.Context, params DeleteInvoiceParams) error {
	return c.post(ctx, "v1/deleteinvoice", params, nil)
}

// SendInvoiceAsEInvoice transmits an already-created Merit invoice as an
// e-invoice over the recipient's configured operator network (Omniva /
// banks / APIX). Routing is read from the Customer record's EInvOperator
// field — the send call itself only takes the invoice SIHId.
//
// Returns the raw response from Merit:
//   - "OK"         → Merit accepted handoff
//   - "api-noeinv" → the receiver is not e-invoice capable
//
// Merit does not document delivery confirmation; this call is
// fire-and-forget from the caller's perspective.
func (c *Client) SendInvoiceAsEInvoice(ctx context.Context, sihID string, deliveryNote bool) (string, error) {
	req := struct {
		Id        string `json:"Id"`
		DelivNote bool   `json:"DelivNote"`
	}{Id: sihID, DelivNote: deliveryNote}
	return c.postRaw(ctx, "v2/sendinvoiceaseinv", req)
}
