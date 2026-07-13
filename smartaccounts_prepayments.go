package accounting

import (
	"context"
	"fmt"

	"github.com/qbitsoftware/accounting-service/smartaccounts"
	"github.com/shopspring/decimal"
)

// SmartAccounts customer-prepayment (ettemaks) support — full park/apply/list
// lifecycle. Confirmed live against a sandbox (see cmd/sa-prepay-probe):
//
//	PARK  — payments:add with a top-level `prepayment` object and NO rows.
//	        SA assigns a paymentId (GUID) — the stable handle for the advance.
//	APPLY — payments:add, amount 0, two POSITIVE rows: a CLIENT_INVOICE row (pays
//	        the invoice) + a PREPAYMENT_PAYMENT row whose id is the parked
//	        payment's id (draws the advance down). Net cash zero.
//	LIST  — clients:getbalance?fetchDetails=true → prepaymentsUnusedList, each
//	        carrying the parked paymentId and the still-unused (negative) amount.
//
// Unlike Excellent Books, SmartAccounts accepts credit notes on fully-paid
// invoices directly, so UnallocateToPrepayment (EB's "free the paid amount
// first" step) is unnecessary and intentionally unsupported.
//
// smartProvider satisfies PrepaymentProvider.
var _ PrepaymentProvider = (*smartProvider)(nil)

// CreatePrepayment parks an unallocated customer advance. input.CustomerCode is
// the SA client id, input.BankID the bank-account NAME the receipt books into,
// and input.Account the GL prepayment account (e.g. "21211"). SA assigns the
// prepayment's id, returned as Prepayment.Number/DocID — the caller stores it to
// reference the advance later in ApplyPrepayment; input.PrepaymentNo is passed
// through as the payment document for traceability only.
func (p *smartProvider) CreatePrepayment(ctx context.Context, input CreatePrepaymentInput) (*Prepayment, error) {
	if input.CustomerCode == "" {
		return nil, p.wrapError("CreatePrepayment", fmt.Errorf("%w: SmartAccounts client id (CustomerCode) is required", ErrInvalidInput))
	}
	if !input.Amount.IsPositive() {
		return nil, p.wrapError("CreatePrepayment", fmt.Errorf("%w: amount must be positive", ErrInvalidInput))
	}

	req := smartaccounts.CreatePaymentRequest{
		Date:        saFormatDate(input.PaymentDate),
		PartnerType: smartaccounts.PartnerClient,
		ClientID:    input.CustomerCode,
		AccountType: smartaccounts.AccountBank,
		AccountName: input.BankID,
		Currency:    input.Currency,
		Amount:      input.Amount,
		Document:    input.PrepaymentNo, // klubio's KLB-PP-<n>, for traceability
		Prepayment: &smartaccounts.PaymentPrepayment{
			Amount:      input.Amount,
			Description: input.Comment,
			Account:     input.Account,
		},
	}
	resp, err := p.client.CreatePayment(ctx, req)
	if err != nil {
		return nil, p.wrapError("CreatePrepayment", err)
	}
	return &Prepayment{
		Number:       resp.PaymentID, // SA-assigned id is the stable handle
		DocID:        resp.PaymentID,
		CustomerCode: input.CustomerCode,
		Amount:       input.Amount,
		Remaining:    input.Amount,
		Currency:     input.Currency,
		Date:         input.PaymentDate,
		Comment:      input.Comment,
	}, nil
}

// ApplyPrepayment ties an existing advance to an invoice with no cash movement:
// a net-zero payment pairing a CLIENT_INVOICE row (pays the invoice) with a
// PREPAYMENT_PAYMENT row that draws down the parked advance referenced by id.
// input.PrepaymentNo is the parked payment's SA id (from CreatePrepayment);
// input.InvoiceNo is the invoice NUMBER, resolved here to its SA id for the row.
func (p *smartProvider) ApplyPrepayment(ctx context.Context, input ApplyPrepaymentInput) error {
	if input.CustomerCode == "" || input.InvoiceNo == "" || input.PrepaymentNo == "" {
		return p.wrapError("ApplyPrepayment", fmt.Errorf("%w: CustomerCode, InvoiceNo and PrepaymentNo are required", ErrInvalidInput))
	}
	if !input.Amount.IsPositive() {
		return p.wrapError("ApplyPrepayment", fmt.Errorf("%w: amount must be positive", ErrInvalidInput))
	}
	inv, err := p.client.FindInvoiceByNumber(ctx, input.InvoiceNo)
	if err != nil {
		return p.wrapError("ApplyPrepayment", fmt.Errorf("resolve invoice %s: %w", input.InvoiceNo, err))
	}

	req := smartaccounts.CreatePaymentRequest{
		Date:        saFormatDate(input.PaymentDate),
		PartnerType: smartaccounts.PartnerClient,
		ClientID:    input.CustomerCode,
		AccountType: smartaccounts.AccountBank,
		AccountName: input.BankID,
		Currency:    input.Currency,
		Amount:      decimal.Zero, // net-zero: the two rows offset each other
		Rows: []smartaccounts.PaymentRow{
			{Type: smartaccounts.RowClientInvoice, ID: inv.ID, Amount: input.Amount},
			{Type: smartaccounts.RowPrepayment, ID: input.PrepaymentNo, Amount: input.Amount},
		},
	}
	_, err = p.client.CreatePayment(ctx, req)
	return p.wrapError("ApplyPrepayment", err)
}

// UnallocateToPrepayment is unsupported on SmartAccounts. Unlike Excellent
// Books, SA accepts credit notes on fully-paid invoices directly, so freeing the
// paid amount into a prepayment first is unnecessary; the naive net-zero
// reversal is also rejected by SA's payment validation.
func (p *smartProvider) UnallocateToPrepayment(_ context.Context, _ UnallocateToPrepaymentInput) (*Prepayment, error) {
	return nil, p.wrapError("UnallocateToPrepayment", ErrUnsupportedProvider)
}

// ListPrepayments returns the client's unallocated advances from the balance
// report. SmartAccounts has no standalone prepayment register and getbalance is
// per-client, so CustomerCode is required. UnusedAmount is negative (a
// liability); Remaining is its magnitude. The original advance amount is not
// separately reported once partly drawn, so Amount mirrors Remaining.
func (p *smartProvider) ListPrepayments(ctx context.Context, input ListPrepaymentsInput) ([]Prepayment, error) {
	if input.CustomerCode == "" {
		// clients:getbalance is per-client; there is no club-wide prepayment
		// register to sweep. Return empty (not an error) so a caller doing a
		// club-wide list degrades gracefully — a per-client sweep must iterate
		// customer codes and call this once per code.
		return nil, nil
	}
	balDate := ""
	if !input.Until.IsZero() {
		balDate = saFormatDate(input.Until)
	}
	bal, err := p.client.GetClientBalance(ctx, input.CustomerCode, balDate, true)
	if err != nil {
		return nil, p.wrapError("ListPrepayments", err)
	}

	out := make([]Prepayment, 0, len(bal.PrepaymentsUnused))
	for _, pp := range bal.PrepaymentsUnused {
		rem := pp.UnusedAmount.Abs()
		if !rem.IsPositive() {
			continue
		}
		out = append(out, Prepayment{
			Number:       pp.PaymentID,
			DocID:        pp.PaymentID,
			CustomerCode: input.CustomerCode,
			Amount:       rem,
			Remaining:    rem,
			Currency:     pp.Currency,
			Date:         saParseDate(pp.PaymentDate),
			Comment:      pp.Description,
		})
	}
	return out, nil
}
