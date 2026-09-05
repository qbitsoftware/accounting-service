package accounting

import (
	"context"
	"fmt"
	"time"

	"github.com/qbitsoftware/accounting-service/merit"
)

// Merit customer-prepayment (ettemaks) support — PUSH, FETCH and APPLY.
//
// PUSH:  CreatePrepayment parks an unallocated customer advance via
//
//	POST v2/Banks/{bankId}/PrePayments/ForCustomer/{customerId}.
//
// FETCH: ListPrepayments reads advances back from the customer-debt report
//
//	(getcustdebtrep), where a prepayment appears as a DocType="BA" line
//	with a negative UnPaidAmount and a stable DocId.
//
// APPLY: ApplyPrepayment offsets an advance against a specific sales invoice
//
//	with a two-line settlement (POST v2/sendsettlement).
//
// APPLY was reported as unsupported here for a long time, on the belief that
// Merit exposes no allocation endpoint and does not auto-apply. The second half
// is true; the first is not. There is no *dedicated* apply endpoint, but a
// settlement (tasaarveldus) does exactly the job, and the reference manual has
// always documented Prepayment as one of the document types a settlement line
// may carry. Verified live in merit/phase0_settle_prepayment_test.go.
//
// The belief was expensive: klubio applied members' credit locally, Merit never
// learned of it, the two ledgers drifted apart, and each application became a
// manual allocation for the club's bookkeeper.
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
			CustomerName: it.PartnerName,
			Amount:       it.TotalAmount.Neg(),
			Remaining:    it.UnPaidAmount.Neg(),
			Currency:     it.CurrencyCode,
			Date:         parseDebtDate(it.DocDate),
		})
	}
	return out
}

// ApplyPrepayment allocates a customer advance to a specific sales invoice by
// posting a two-line settlement (tasaarveldus): the invoice positive, the
// prepayment negative, totalling zero.
//
// This was reported as unsupported for a long time, on the belief that Merit
// offers no allocation endpoint. It does — just not a dedicated one. The cost of
// that belief was real: klubio spent members' credit locally, Merit never
// learned, the two ledgers drifted, and every application became a manual
// allocation for the club's bookkeeper. See merit/settlements.go for the live
// verification.
//
// input.PrepaymentNo is the advance's DocumentNumber and input.InvoiceNo the
// sales invoice's number — both are document numbers here, NOT the GUIDs used
// elsewhere in this adapter.
func (p *meritProvider) ApplyPrepayment(ctx context.Context, input ApplyPrepaymentInput) error {
	if input.InvoiceNo == "" || input.PrepaymentNo == "" {
		return p.wrapError("ApplyPrepayment", fmt.Errorf("both InvoiceNo and PrepaymentNo are required"))
	}
	if !input.Amount.IsPositive() {
		return p.wrapError("ApplyPrepayment", fmt.Errorf("amount must be positive, got %s", input.Amount))
	}
	date := input.PaymentDate
	if date.IsZero() {
		date = time.Now().UTC()
	}
	_, err := p.client.SendSettlement(ctx, merit.SettlementRequest{
		DocDate:      date.Format(meritDateFormat),
		CurrencyCode: input.Currency,
		CustLines: []merit.SettlementLine{
			{CustVendID: input.CustomerCode, DocNo: input.InvoiceNo, Amount: input.Amount},
			{CustVendID: input.CustomerCode, DocNo: input.PrepaymentNo, Amount: input.Amount.Neg()},
		},
	})
	return p.wrapError("ApplyPrepayment", err)
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
