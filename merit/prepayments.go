package merit

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// CreatePrepaymentRequest is the body for the PrePayments/ForCustomer endpoint.
// NOTE: DocumentDate here is yyyy-MM-dd (this endpoint differs from the
// yyyyMMdd used by sendinvoice/getpayments — Merit is inconsistent).
type CreatePrepaymentRequest struct {
	Description    string          `json:"Description,omitempty"`
	DocumentNumber string          `json:"DocumentNumber,omitempty"`
	CurrencyCode   string          `json:"CurrencyCode,omitempty"`
	DocumentDate   string          `json:"DocumentDate"`
	Amount         decimal.Decimal `json:"Amount"`
}

// CreatePrepaymentResponse is the batch acknowledgement Merit returns.
type CreatePrepaymentResponse struct {
	BatchInfo string `json:"BatchInfo"`
	BatchId   string `json:"BatchId"`
}

// CreateCustomerPrepayment records an unallocated customer advance (ettemaks)
// against a bank account. Verified path:
//
//	POST v2/Banks/{bankId}/PrePayments/ForCustomer/{customerId}
//
// bankID and customerID are Merit GUIDs. There is no flat sendprepayment
// endpoint; the customerId is a path segment (the ?customerId= form 404s).
func (c *Client) CreateCustomerPrepayment(ctx context.Context, bankID, customerID string, req CreatePrepaymentRequest) (*CreatePrepaymentResponse, error) {
	if bankID == "" || customerID == "" {
		return nil, fmt.Errorf("merit: bankID and customerID are required for prepayment")
	}
	var result CreatePrepaymentResponse
	endpoint := fmt.Sprintf("v2/Banks/%s/PrePayments/ForCustomer/%s", bankID, customerID)
	if err := c.post(ctx, endpoint, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
