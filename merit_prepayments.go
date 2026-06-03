package accounting

import (
	"context"
	"time"

	"github.com/qbitsoftware/accounting-service/merit"
)

// Merit customer-prepayment (ettemaks) support — PUSH + FETCH only (v1).
//
// PUSH:  CreatePrepayment parks an unallocated customer advance via
//        POST v2/Banks/{bankId}/PrePayments/ForCustomer/{customerId}.
// FETCH: ListPrepayments reads advances back from the customer-debt report
//        (getcustdebtrep), where a prepayment appears as a DocType="BA" line
//        with a negative UnPaidAmount and a stable DocId.
//
// APPLY is intentionally NOT supported: Merit has no confirmed API to allocate
// a prepayment to a specific invoice and does not auto-apply (verified live).
// klubio keeps apply EB-only and leaves Merit allocation to the accountant.
// See docs/MERIT_PREPAYMENT_PLAN.md (SCOPE DECISION).
//
// meritProvider satisfies PrepaymentProvider.
var _ PrepaymentProvider = (*meritProvider)(nil)

// CreatePrepayment records an unallocated customer advance. For Merit,
// input.CustomerCode is the customer GUID, input.BankID is the bank GUID, and
// input.PrepaymentNo becomes the prepayment DocumentNumber.
func (p *meritProvider) CreatePrepayment(ctx context.Context, input CreatePrepaymentInput) (*Prepayment, error) {
	resp, err := p.client.CreateCustomerPrepayment(ctx, input.BankID, input.CustomerCode, merit.CreatePrepaymentRequest{
		Description:    input.Comment,
		DocumentNumber: input.PrepaymentNo,
		CurrencyCode:   input.Currency,
		DocumentDate:   input.PaymentDate.Format("2006-01-02"), // this endpoint wants yyyy-MM-dd
		Amount:         input.Amount,
	})
	if err != nil {
		return nil, p.wrapError("CreatePrepayment", err)
	}
	return &Prepayment{
		Number:       input.PrepaymentNo,
		DocID:        resp.BatchId,
		CustomerCode: input.CustomerCode,
		Amount:       input.Amount,
		Remaining:    input.Amount,
		Currency:     input.Currency,
		Date:         input.PaymentDate,
		Comment:      input.Comment,
	}, nil
}

// ListPrepayments returns the customer's open advances derived from the
// customer-debt report. An advance is any line with a negative UnPaidAmount
// (DocType "BA"); Remaining is the still-unapplied credit (the report nets
// applications as of DebtDate), so no separate balance read is needed.
func (p *meritProvider) ListPrepayments(ctx context.Context, input ListPrepaymentsInput) ([]Prepayment, error) {
	params := merit.CustomerDebtsParams{CustID: input.CustomerCode}
	if !input.Until.IsZero() {
		params.DebtDate = input.Until.Format(meritDateFormat)
	}
	items, err := p.client.CustomerDebts(ctx, params)
	if err != nil {
		return nil, p.wrapError("ListPrepayments", err)
	}
	return meritAdvancesFromDebts(items), nil
}

// meritAdvancesFromDebts maps customer-debt-report rows to prepayments. Pure, so
// the document-type filtering is unit-tested without the network.
//
// An advance is a row with DocType "BA" (ettemaks) and a negative UnPaidAmount.
// Credit notes also show as negative lines but carry DocType "MA" — they are NOT
// ettemaks and must be excluded, or importing them would double-count the credit
// klubio already booked when the credit note was issued.
func meritAdvancesFromDebts(items []merit.CustomerDebtItem) []Prepayment {
	var out []Prepayment
	for _, it := range items {
		if it.DocType != "BA" {
			continue
		}
		if !it.UnPaidAmount.IsNegative() {
			continue // positive = a debt, not a credit
		}
		out = append(out, Prepayment{
			Number:       it.DocNo,
			DocID:        it.DocID,
			CustomerCode: it.PartnerID,
			Amount:       it.TotalAmount.Neg(),
			Remaining:    it.UnPaidAmount.Neg(),
			Currency:     it.CurrencyCode,
			Date:         parseDebtDate(it.DocDate),
		})
	}
	return out
}

// ApplyPrepayment is not supported on Merit (v1). See SCOPE DECISION.
func (p *meritProvider) ApplyPrepayment(ctx context.Context, input ApplyPrepaymentInput) error {
	return p.wrapError("ApplyPrepayment", ErrUnsupportedProvider)
}

// UnallocateToPrepayment is not supported on Merit (v1). See SCOPE DECISION.
func (p *meritProvider) UnallocateToPrepayment(ctx context.Context, input UnallocateToPrepaymentInput) (*Prepayment, error) {
	return nil, p.wrapError("UnallocateToPrepayment", ErrUnsupportedProvider)
}

// parseDebtDate parses the ISO timestamps returned by getcustdebtrep
// ("2025-06-02T00:00:00"), distinct from the yyyyMMdd used elsewhere.
func parseDebtDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return t
	}
	return parseDate(s)
}
