package merit

import (
	"context"

	"github.com/shopspring/decimal"
)

// Settlements (tasaarveldus) offset a customer's documents against one another
// without any money moving. This is how a customer PREPAYMENT is applied to a
// specific sales invoice in Merit — there is no dedicated "apply prepayment"
// endpoint, which is why the adapter reported the operation as unsupported for
// a long time.
//
// Verified live (merit/phase0_settle_prepayment_test.go): an advance of 100 and
// an unpaid invoice of 100 for the same customer, settled in one call, left the
// invoice at PaidAmount=100 / Paid=true and removed both from the open-debt
// report.
//
//	POST v2/sendsettlement
//
// The sign convention comes from the document type, per the reference manual:
// SalesInvoice and PurchaseInvoice are POSITIVE, CreditInvoice and Prepayment
// are NEGATIVE — and every line in the payload must total exactly zero.

// SettlementLine is one document taking part in a settlement. DocNo is the
// document's own number: the invoice number for a sales invoice, the
// DocumentNumber the advance was created with for a prepayment.
type SettlementLine struct {
	CustVendName  string          `json:"CustVendName,omitempty"`
	CustVendID    string          `json:"CustVendId,omitempty"`
	CustVendRegNo string          `json:"CustVendRegNo,omitempty"`
	DocNo         string          `json:"DocNo"`
	Amount        decimal.Decimal `json:"Amount"`
}

// SettlementRequest is the body for v2/sendsettlement. DocDate is yyyyMMdd,
// like sendinvoice and unlike PrePayments/ForCustomer.
type SettlementRequest struct {
	DocDate      string           `json:"DocDate"`
	CurrencyCode string           `json:"CurrencyCode,omitempty"`
	CustLines    []SettlementLine `json:"CustLines"`
	VendLines    []SettlementLine `json:"VendLines"`
}

// SettlementResponse is Merit's acknowledgement of a posted settlement.
type SettlementResponse struct {
	DocumentID string `json:"DocumentId"`
	DocumentNo string `json:"DocumentNo"`
}

// SendSettlement posts a settlement. Callers are responsible for the lines
// summing to zero — Merit rejects the document otherwise, and balancing it here
// would mean guessing which side the caller got wrong.
func (c *Client) SendSettlement(ctx context.Context, req SettlementRequest) (*SettlementResponse, error) {
	// Merit rejects a null array where it expects a list, so never send one.
	if req.CustLines == nil {
		req.CustLines = []SettlementLine{}
	}
	if req.VendLines == nil {
		req.VendLines = []SettlementLine{}
	}
	var resp SettlementResponse
	if err := c.post(ctx, "v2/sendsettlement", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
