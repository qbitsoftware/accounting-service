package merit

import "context"

// ListPayments retrieves a list of payments for the given period.
// The period may not span more than 3 months.
func (c *Client) ListPayments(ctx context.Context, params ListPaymentsParams) ([]PaymentListItem, error) {
	var result []PaymentListItem
	if err := c.post(ctx, "v2/getpayments", params, &result); err != nil {
		return nil, err
	}
	return result, nil
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
