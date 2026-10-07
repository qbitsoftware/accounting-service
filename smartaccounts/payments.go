package smartaccounts

import (
	"context"
	"fmt"
	"net/url"
	"time"

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

	// The fields below were added in v0.15.0 for the receipt-level reads. All of
	// them zero puts exactly the v0.14.5 query on the wire (encodeQuery sorts the
	// keys, so appending them here cannot reorder anything already sent).
	ID          string
	AccountName string
	PartnerName string
	// FetchComments asks SmartAccounts to include each payment's comment.
	FetchComments bool
	// DateFromPrecise, when non-zero, replaces DateFrom with a second-precision
	// "dd.MM.yyyy_HH:mm:ss" in Europe/Tallinn — the feed cursor. SANDBOX (checklist
	// item 6): that SA honours the time part, and filters in Tallinn time.
	DateFromPrecise time.Time
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
	setNonEmpty(v, "id", p.ID)
	setNonEmpty(v, "accountName", p.AccountName)
	setNonEmpty(v, "partnerName", p.PartnerName)
	if p.FetchComments {
		v.Set("fetchComments", "true")
	}
	if !p.DateFromPrecise.IsZero() {
		v.Set("dateFrom", tallinnTime(p.DateFromPrecise).Format(preciseDateFormat))
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

// NettingComment is the comment SettleInvoiceAgainstCredit writes on the netting
// payment. It is how a reader recognises that payment as netting rather than
// money — the bytes are the ones v0.14.5 has always sent, so payments already in
// SmartAccounts carry it too.
const NettingComment = "auto-settle credit against original"

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
		Comment:     NettingComment,
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
