package accounting

import (
	"context"
	"fmt"

	"github.com/qbitsoftware/accounting-service/excellentbooks"
	"github.com/shopspring/decimal"
)

// Excellent Books prepayment (ettemaks) support. EB models a prepayment as a
// confirmed receipt (IPVc) row carrying a CUPNr (prepayment number) and no
// InvoiceNr. Applying or freeing a prepayment is a two-row receipt where the
// invoice row and the CUPNr row carry opposite signs; net cash is zero. All of
// this was confirmed against a live EB test instance (see cmd/eb-prepay-probe).
//
// excellentProvider satisfies PrepaymentProvider.
var _ PrepaymentProvider = (*excellentProvider)(nil)

// CreatePrepayment records an unallocated customer advance.
func (p *excellentProvider) CreatePrepayment(ctx context.Context, input CreatePrepaymentInput) (*Prepayment, error) {
	if input.BankID == "" {
		return nil, p.wrapError("CreatePrepayment", fmt.Errorf("PayMode (BankID) is required for Excellent Books receipts"))
	}
	if input.PrepaymentNo == "" {
		return nil, p.wrapError("CreatePrepayment", fmt.Errorf("PrepaymentNo (CUPNr) is required"))
	}

	fields := map[string]string{
		"set_field.TransDate": formatExcellentDate(input.PaymentDate),
		"set_field.OKFlag":    "1",
		"set_field.PayMode":   input.BankID,
	}
	if input.Currency != "" {
		fields["set_field.PayCurCode"] = input.Currency
	}
	fields["set_row_field.0.stp"] = "1"
	fields["set_row_field.0.CustCode"] = input.CustomerCode
	fields["set_row_field.0.CUPNr"] = input.PrepaymentNo
	fields["set_row_field.0.RecVal"] = input.Amount.String()
	fields["set_row_field.0.PayDate"] = formatExcellentDate(input.PaymentDate)
	if input.Comment != "" {
		fields["set_row_field.0.Comment"] = input.Comment
	}

	if _, err := p.client.CreateReceipt(ctx, fields); err != nil {
		return nil, p.wrapError("CreatePrepayment", err)
	}
	return &Prepayment{
		Number:       input.PrepaymentNo,
		CustomerCode: input.CustomerCode,
		Amount:       input.Amount,
		Remaining:    input.Amount,
		Currency:     input.Currency,
		Date:         input.PaymentDate,
		Comment:      input.Comment,
	}, nil
}

// ApplyPrepayment settles (part of) an invoice from an existing prepayment.
// Two rows, net cash zero: invoice row positive, prepayment row negative.
func (p *excellentProvider) ApplyPrepayment(ctx context.Context, input ApplyPrepaymentInput) error {
	if input.BankID == "" {
		return p.wrapError("ApplyPrepayment", fmt.Errorf("PayMode (BankID) is required for Excellent Books receipts"))
	}
	if input.InvoiceNo == "" || input.PrepaymentNo == "" {
		return p.wrapError("ApplyPrepayment", fmt.Errorf("InvoiceNo and PrepaymentNo are required"))
	}

	date := formatExcellentDate(input.PaymentDate)
	fields := map[string]string{
		"set_field.TransDate": date,
		"set_field.OKFlag":    "1",
		"set_field.PayMode":   input.BankID,
		// row0: pay the invoice
		"set_row_field.0.stp":       "1",
		"set_row_field.0.CustCode":  input.CustomerCode,
		"set_row_field.0.InvoiceNr": input.InvoiceNo,
		"set_row_field.0.RecVal":    input.Amount.String(),
		"set_row_field.0.PayDate":   date,
		// row1: draw the same amount from the prepayment (negative)
		"set_row_field.1.stp":      "1",
		"set_row_field.1.CustCode": input.CustomerCode,
		"set_row_field.1.CUPNr":    input.PrepaymentNo,
		"set_row_field.1.RecVal":   input.Amount.Neg().String(),
		"set_row_field.1.PayDate":  date,
	}
	if input.Currency != "" {
		fields["set_field.PayCurCode"] = input.Currency
	}

	_, err := p.client.CreateReceipt(ctx, fields)
	return p.wrapError("ApplyPrepayment", err)
}

// UnallocateToPrepayment frees a paid amount off an invoice into a new
// prepayment, re-opening the invoice. Two rows, net cash zero: invoice row
// negative, new prepayment row positive.
func (p *excellentProvider) UnallocateToPrepayment(ctx context.Context, input UnallocateToPrepaymentInput) (*Prepayment, error) {
	if input.BankID == "" {
		return nil, p.wrapError("UnallocateToPrepayment", fmt.Errorf("PayMode (BankID) is required for Excellent Books receipts"))
	}
	if input.InvoiceNo == "" || input.PrepaymentNo == "" {
		return nil, p.wrapError("UnallocateToPrepayment", fmt.Errorf("InvoiceNo and PrepaymentNo are required"))
	}

	date := formatExcellentDate(input.PaymentDate)
	fields := map[string]string{
		"set_field.TransDate": date,
		"set_field.OKFlag":    "1",
		"set_field.PayMode":   input.BankID,
		// row0: un-pay the invoice (negative) — re-opens it by this amount
		"set_row_field.0.stp":       "1",
		"set_row_field.0.CustCode":  input.CustomerCode,
		"set_row_field.0.InvoiceNr": input.InvoiceNo,
		"set_row_field.0.RecVal":    input.Amount.Neg().String(),
		"set_row_field.0.PayDate":   date,
		// row1: park the freed amount as a new prepayment (positive)
		"set_row_field.1.stp":      "1",
		"set_row_field.1.CustCode": input.CustomerCode,
		"set_row_field.1.CUPNr":    input.PrepaymentNo,
		"set_row_field.1.RecVal":   input.Amount.String(),
		"set_row_field.1.PayDate":  date,
	}
	if input.Currency != "" {
		fields["set_field.PayCurCode"] = input.Currency
	}
	if input.Comment != "" {
		fields["set_row_field.0.Comment"] = input.Comment
	}

	if _, err := p.client.CreateReceipt(ctx, fields); err != nil {
		return nil, p.wrapError("UnallocateToPrepayment", err)
	}
	return &Prepayment{
		Number:       input.PrepaymentNo,
		CustomerCode: input.CustomerCode,
		Amount:       input.Amount,
		Remaining:    input.Amount,
		Currency:     input.Currency,
		Date:         input.PaymentDate,
		Comment:      input.Comment,
	}, nil
}

// ListPrepayments aggregates receipt rows carrying a CUPNr into per-prepayment
// balances. Remaining is the net of all rows for a CUPNr (advances positive,
// draws negative); Amount is the sum of the positive (advance) rows. EB does
// not expose a prepayment register directly, so this is derived from receipts
// in the window.
func (p *excellentProvider) ListPrepayments(ctx context.Context, input ListPrepaymentsInput) ([]Prepayment, error) {
	params := excellentbooks.ListParams{Limit: 5000}
	if !input.Since.IsZero() {
		params.Sort = "TransDate"
		params.Range = formatExcellentDate(input.Since) + ":"
		if !input.Until.IsZero() {
			params.Range += formatExcellentDate(input.Until)
		}
	}

	receipts, _, err := p.client.ListReceipts(ctx, params)
	if err != nil {
		return nil, p.wrapError("ListPrepayments", err)
	}

	type agg struct {
		pp    Prepayment
		order int
	}
	byCUPNr := make(map[string]*agg)
	next := 0
	for _, r := range receipts {
		for _, row := range r.Rows {
			if row.CUPNr == "" {
				continue
			}
			if input.CustomerCode != "" && row.CustCode != input.CustomerCode {
				continue
			}
			val, _ := decimal.NewFromString(row.RecVal)
			a, ok := byCUPNr[row.CUPNr]
			if !ok {
				a = &agg{order: next, pp: Prepayment{
					Number:       row.CUPNr,
					CustomerCode: row.CustCode,
					Currency:     r.PayCurCode,
					Date:         parseExcellentDate(r.TransDate),
					Comment:      row.Comment,
				}}
				byCUPNr[row.CUPNr] = a
				next++
			}
			a.pp.Remaining = a.pp.Remaining.Add(val)
			if val.IsPositive() {
				a.pp.Amount = a.pp.Amount.Add(val)
			}
		}
	}

	out := make([]Prepayment, len(byCUPNr))
	for _, a := range byCUPNr {
		out[a.order] = a.pp
	}
	return out, nil
}
