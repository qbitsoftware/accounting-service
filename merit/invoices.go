package merit

import (
	"context"
	"time"
)

// ListInvoices retrieves a list of sales invoices for the given period.
//
// The period may not span more than 3 months — Merit answers a wider one with
// 400 "Periood liiga pikk(max 3 kuud)". This is the raw single-call form; use
// ListInvoicesRange to fetch an arbitrarily long range, including history far
// older than three months.
func (c *Client) ListInvoices(ctx context.Context, params ListInvoicesParams) ([]InvoiceListItem, error) {
	var result []InvoiceListItem
	if err := c.post(ctx, "v2/getinvoices", params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListInvoicesRange retrieves sales invoices over any range, splitting it into
// as many legal calls as Merit's 3-month cap requires and merging the results.
//
// Only PeriodStart/PeriodEnd are derived from the range; every other field of
// params (UnPaid, DateType) is applied to each call unchanged. Results are
// deduplicated by SIHId.
//
// A failure in one period does not discard the others: the invoices that were
// fetched come back together with a *PartialRangeError naming what was missed.
func (c *Client) ListInvoicesRange(ctx context.Context, start, end time.Time, params ListInvoicesParams) ([]InvoiceListItem, error) {
	periods := SplitPeriod(start, end)
	if len(periods) == 0 {
		return nil, nil
	}
	items, err := fetchRange(ctx, periods, func(ctx context.Context, p Period) ([]InvoiceListItem, error) {
		q := params
		q.PeriodStart = FormatDate(p.Start)
		q.PeriodEnd = FormatDate(p.End)
		return c.ListInvoices(ctx, q)
	})
	return dedupeBy(items, func(i InvoiceListItem) string { return i.SIHId }), err
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
