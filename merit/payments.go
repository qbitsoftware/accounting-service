package merit

import (
	"context"
	"time"
)

// ListPayments retrieves a list of payments for the given period.
//
// The period may not span more than 3 months — Merit answers a wider one with
// 400 "Periood liiga pikk(max 3 kuud)". This is the raw single-call form; use
// ListPaymentsRange to fetch an arbitrarily long range.
func (c *Client) ListPayments(ctx context.Context, params ListPaymentsParams) ([]PaymentListItem, error) {
	var result []PaymentListItem
	if err := c.post(ctx, "v2/getpayments", params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListPaymentsRange retrieves payments over any range, splitting it into as many
// legal calls as Merit's 3-month cap requires and merging the results.
//
// Only PeriodStart/PeriodEnd are derived from the range; every other field of
// params (PaymentType, BankID, DateType) is applied to each call unchanged.
// Results are deduplicated by ID().
//
// A failure in one period does not discard the others: the payments that were
// fetched come back together with a *PartialRangeError naming what was missed.
func (c *Client) ListPaymentsRange(ctx context.Context, start, end time.Time, params ListPaymentsParams) ([]PaymentListItem, error) {
	periods := SplitPeriod(start, end)
	if len(periods) == 0 {
		return nil, nil
	}
	items, err := fetchRange(ctx, periods, func(ctx context.Context, p Period) ([]PaymentListItem, error) {
		q := params
		q.PeriodStart = FormatDate(p.Start)
		q.PeriodEnd = FormatDate(p.End)
		return c.ListPayments(ctx, q)
	})
	return dedupeBy(items, func(i PaymentListItem) string { return i.ID() }), err
}

// CreatePayment creates a payment for a sales invoice.
func (c *Client) CreatePayment(ctx context.Context, req CreatePaymentRequest) error {
	return c.post(ctx, "v2/sendpayment", req, nil)
}

// CreatePurchasePayment creates a payment for a purchase invoice.
func (c *Client) CreatePurchasePayment(ctx context.Context, req CreatePurchasePaymentRequest) error {
	return c.post(ctx, "v2/sendPaymentV", req, nil)
}

// DeletePayment deletes a payment by ID.
func (c *Client) DeletePayment(ctx context.Context, params DeletePaymentParams) error {
	return c.post(ctx, "v1/deletepayment", params, nil)
}
