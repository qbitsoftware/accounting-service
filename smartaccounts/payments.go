package smartaccounts

import (
	"context"
	"fmt"
	"net/url"

	"github.com/shopspring/decimal"
)

// ListPaymentsParams filters a payments:get query.
type ListPaymentsParams struct {
	DateFrom    string // dd.MM.yyyy
	DateTo      string // dd.MM.yyyy
	DateType    string // "", "date", "modifydate"
	PartnerType string // "", "CLIENT", "VENDOR"
	AccountType string // "", "BANK", "CASH"
	PartnerID   string
	FetchRows   bool
}

func (p ListPaymentsParams) values() url.Values {
	v := url.Values{}
	setNonEmpty(v, "dateFrom", p.DateFrom)
	setNonEmpty(v, "dateTo", p.DateTo)
	setNonEmpty(v, "dateType", p.DateType)
	setNonEmpty(v, "partnerType", p.PartnerType)
	setNonEmpty(v, "accountType", p.AccountType)
	setNonEmpty(v, "partnerId", p.PartnerID)
	if p.FetchRows {
		v.Set("fetchRows", "true")
	}
	return v
}

// ListPayments retrieves payments matching params, following pagination. The
// second return value holds IDs deleted since the queried modifydate.
func (c *Client) ListPayments(ctx context.Context, params ListPaymentsParams) ([]PaymentItem, []string, error) {
	var items []PaymentItem
	deleted, err := c.getList(ctx, "purchasesales/payments:get", params.values(), &items)
	if err != nil {
		return nil, nil, err
	}
	return items, deleted, nil
}

// CreatePayment adds a payment.
func (c *Client) CreatePayment(ctx context.Context, req CreatePaymentRequest) (*PaymentResponse, error) {
	var resp PaymentResponse
	if err := c.post(ctx, "purchasesales/payments:add", nil, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SettleInvoiceAgainstCredit posts a netting payment that closes an original
// client invoice against a linked credit invoice, applying `settle` to each
// side. Both row amounts are sent POSITIVE: SA validates each row against its
// invoice's outstanding using magnitude, and rejects negative-row attempts on
// credits ("payment amount larger than outstanding amount on row"). The total
// payment.amount is 2 × settle, conceptually +settle in from the original
// payer and +settle out to the credit recipient — net zero on the netting bank.
//
// Caller must have c.NettingBank() configured; otherwise this returns an error.
// The two invoices must be for the same client and currency.
func (c *Client) SettleInvoiceAgainstCredit(ctx context.Context, clientID, originalID, creditID string, settle decimal.Decimal, currency, date string) (*PaymentResponse, error) {
	if c.nettingBank == "" {
		return nil, fmt.Errorf("smartaccounts: NettingBank not configured; cannot auto-settle")
	}
	if !settle.IsPositive() {
		return nil, fmt.Errorf("smartaccounts: settle amount must be positive, got %s", settle)
	}
	req := CreatePaymentRequest{
		Date:        date,
		PartnerType: PartnerClient,
		ClientID:    clientID,
		AccountType: AccountBank,
		AccountName: c.nettingBank,
		Currency:    currency,
		Amount:      settle.Add(settle),
		Comment:     "auto-settle credit against original",
		Rows: []PaymentRow{
			{Type: RowClientInvoice, ID: originalID, Amount: settle},
			{Type: RowClientInvoice, ID: creditID, Amount: settle},
		},
	}
	return c.CreatePayment(ctx, req)
}

// DeletePayment deletes a payment by ID. SA's :delete wants id in the URL query
// AND a non-empty POST body — see DeleteInvoice for the same gotcha.
func (c *Client) DeletePayment(ctx context.Context, id string) error {
	v := url.Values{}
	v.Set("id", id)
	return c.post(ctx, "purchasesales/payments:delete", v, struct{}{}, nil)
}
