package merit

import (
	"context"
	"time"
)

// ListPurchases retrieves a list of purchase invoices for the given period.
// The period may not span more than 3 months.
func (c *Client) ListPurchases(ctx context.Context, params ListPurchasesParams) ([]PurchaseListItem, error) {
	var result []PurchaseListItem
	if err := c.post(ctx, "v2/getpurchorders", params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetPurchase retrieves detailed information for a single purchase invoice.
func (c *Client) GetPurchase(ctx context.Context, params GetInvoiceParams) (*InvoiceDetail, error) {
	var result InvoiceDetail
	if err := c.post(ctx, "v2/getpurchorder", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CreatePurchase creates a new purchase invoice.
func (c *Client) CreatePurchase(ctx context.Context, req CreatePurchaseRequest) (*CreatePurchaseResponse, error) {
	var result CreatePurchaseResponse
	if err := c.post(ctx, "v2/sendpurchinvoice", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeletePurchase deletes a purchase invoice by ID.
func (c *Client) DeletePurchase(ctx context.Context, params DeletePurchaseParams) error {
	return c.post(ctx, "v1/deletepurchinvoice", params, nil)
}

// ListPurchasesRange retrieves purchase invoices over any range, splitting it
// into as many legal calls as Merit's 3-month cap requires and merging the
// results. Deduplicated by PIHId; see ListInvoicesRange for the error contract.
func (c *Client) ListPurchasesRange(ctx context.Context, start, end time.Time, params ListPurchasesParams) ([]PurchaseListItem, error) {
	periods := SplitPeriod(start, end)
	if len(periods) == 0 {
		return nil, nil
	}
	items, err := fetchRange(ctx, periods, func(ctx context.Context, p Period) ([]PurchaseListItem, error) {
		q := params
		q.PeriodStart = FormatDate(p.Start)
		q.PeriodEnd = FormatDate(p.End)
		return c.ListPurchases(ctx, q)
	})
	return dedupeBy(items, func(i PurchaseListItem) string { return i.PIHId }), err
}
