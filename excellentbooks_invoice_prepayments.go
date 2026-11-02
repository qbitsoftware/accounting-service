package accounting

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/qbitsoftware/accounting-service/excellentbooks"
)

// Prepayments applied on the invoice itself (WithInvoicePrepayments).
//
// An Excellent Books accountant settles an invoice from a customer's
// prepayment in one of two ways. Through a receipt: an IPVc row on the invoice
// (+X) beside a row drawing the prepayment (CUPNr, −X). Or on the invoice: a
// row of type 6 ("Ettemaks") carrying the CUPNr and the amount drawn, which
// leaves the invoice total (Sum4) as it was and closes the invoice in the
// sales ledger (ARVc, "Arve hetkeseis") all the same. The second never shows
// in IPVc, so a caller reading only receipts sees a paid invoice as open.
//
// Observed on Excellent's test company (02.11.2026, invoices 210009, 240003,
// 250015, 260003): the row is {stp:"6", CUPNr, Sum (positive, VAT included),
// SalesAcc (the prepayment account), Spec:"Ettemaks", Recepy:"dd.mm.yyyy"};
// one CUPNr may be drawn on several invoices; a partly drawn invoice keeps
// the rest open.
//
// Each (invoice, CUPNr) becomes one receipt-shaped record — the shape the
// receipt that did the same would have — so a caller classifies both ways
// alike: Rows {invoice +sum, CUPNr −sum}, Amount zero (the rows offset each
// other, as ApplyPrepaymentReceipt reports), Draft from the invoice's OKFlag,
// Void from its Invalid flag, Date its TransDate. InvoicePrepayment marks it,
// and its ID "IV<SerNr>-<CUPNr>" is no IPVc SerNr.

const (
	// ebRowTypeInvoicePrepayment is the IVVc row type (stp) of a prepayment
	// drawn on the invoice.
	ebRowTypeInvoicePrepayment = "6"
	// ebInvTypeCredit is a credit note's IVVc InvType; credit notes are never
	// read for prepayments.
	ebInvTypeCredit = "3"
	// ebInvoicePrepaymentFields is all an invoice-prepayment read asks of IVVc:
	// enough to place each prepayment row, nothing else (header and row field
	// names share the one fields list).
	ebInvoicePrepaymentFields = "SerNr,TransDate,InvDate,CustCode,OKFlag,Invalid,InvType,CurncyCode,stp,CUPNr,Sum"
)

// EBInvoicePrepaymentID is the ID of the record for prepayment cupnr drawn on
// invoice serNr. "IV" keeps it apart from every IPVc SerNr, which is a number;
// "-" rather than "/" because callers put receipt ids in URL paths.
func EBInvoicePrepaymentID(serNr, cupnr string) string { return "IV" + serNr + "-" + cupnr }

// ebInvoicePrepayments maps one invoice's prepayment rows to receipt-shaped
// records, one per CUPNr in row order. A credit note, or an invoice without
// prepayment rows, gives none. A row whose Sum cannot be read, or that names
// no CUPNr, still gives a record — with DecodeError set, so it is reported and
// never taken as money.
func ebInvoicePrepayments(inv excellentbooks.Invoice) []Receipt {
	if inv.InvType == ebInvTypeCredit || inv.SerNr == "" {
		return nil
	}
	type draw struct {
		sum      decimal.Decimal
		problems []string
	}
	draws := map[string]*draw{}
	var order []string
	for i, row := range inv.Rows {
		if row.Stp != ebRowTypeInvoicePrepayment {
			continue
		}
		cupnr := strings.TrimSpace(row.CUPNr)
		d, ok := draws[cupnr]
		if !ok {
			d = &draw{}
			draws[cupnr] = d
			order = append(order, cupnr)
		}
		name := row.RowNumber
		if name == "" {
			name = strconv.Itoa(i)
		}
		if cupnr == "" {
			d.problems = append(d.problems, fmt.Sprintf("prepayment row %s names no prepayment (CUPNr)", name))
		}
		amt, err := decimal.NewFromString(strings.TrimSpace(row.Sum))
		if err != nil {
			d.problems = append(d.problems, fmt.Sprintf("prepayment row %s: unreadable Sum %q", name, row.Sum))
			continue
		}
		d.sum = d.sum.Add(amt)
	}
	if len(order) == 0 {
		return nil
	}

	date := parseExcellentDate(inv.TransDate)
	if date.IsZero() {
		date = parseExcellentDate(inv.InvDate)
	}
	var problems []string
	void := false
	switch flag := strings.TrimSpace(string(inv.Invalid)); {
	case strings.HasPrefix(flag, "{") || strings.HasPrefix(flag, "["):
		problems = append(problems, fmt.Sprintf("Invalid has an unexpected shape %s", flag))
	case flag != "" && flag != "0":
		void = true
	}

	out := make([]Receipt, 0, len(order))
	for _, cupnr := range order {
		d := draws[cupnr]
		id := EBInvoicePrepaymentID(inv.SerNr, cupnr)
		r := Receipt{
			ID:                id,
			Number:            id,
			Date:              date,
			Currency:          inv.CurncyCode,
			CustomerID:        inv.CustCode,
			Draft:             inv.OKFlag != "1",
			Void:              void,
			InvoicePrepayment: true,
			Rows: []ReceiptRow{
				{Type: saRowClientInvoice, DocID: inv.SerNr, Amount: d.sum, Description: "Ettemaks"},
				{Type: ebRowPrepayment, DocID: cupnr, Amount: d.sum.Neg(), Description: "Ettemaks"},
			},
		}
		switch {
		case d.sum.IsPositive() && cupnr != "":
			r.AdvanceSourceIDs = []string{cupnr}
		case d.sum.IsNegative():
			r.AdvanceAmount = d.sum.Neg()
		}
		if all := append(append([]string(nil), problems...), d.problems...); len(all) > 0 {
			r.DecodeError = strings.Join(all, "; ")
		}
		out = append(out, r)
	}
	return out
}

// readInvoicePrepayments reads the invoices dated since..until (until zero ⇒
// open-ended; filter narrows by header fields, e.g. CustCode) with only the
// fields a prepayment row needs, page by page as readReceiptWindow does, and
// returns their prepayment records. Usable or nothing: an unfinished read
// (maxPages) is ErrTooManyPages, pages that do not join are ErrPageShifted
// (after one fresh attempt).
func (p *excellentProvider) readInvoicePrepayments(ctx context.Context, filter map[string]string, since, until time.Time, maxPages int) ([]Receipt, error) {
	rng := formatExcellentDate(since) + ":"
	if !until.IsZero() {
		rng += formatExcellentDate(until)
	}
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		var invs []excellentbooks.Invoice
		invs, err = p.readInvoiceWindowOnce(ctx, filter, rng, maxPages)
		if err == nil {
			var out []Receipt
			for _, inv := range invs {
				out = append(out, ebInvoicePrepayments(inv)...)
			}
			return out, nil
		}
		if !errors.Is(err, ErrPageShifted) {
			return nil, err
		}
		slog.Warn("excellentbooks: invoice list shifted between pages; reading it again", "range", rng, "attempt", attempt+1, "error", err)
	}
	return nil, err
}

func (p *excellentProvider) readInvoiceWindowOnce(ctx context.Context, filter map[string]string, rng string, maxPages int) ([]excellentbooks.Invoice, error) {
	size := p.receiptPageSize()
	var out []excellentbooks.Invoice
	seen := map[string]int{}
	anchor := ""
	offset := 0
	for page := 0; ; page++ {
		if page == maxPages {
			return nil, fmt.Errorf("%w: IVVc range %s unfinished after %d pages of %d", ErrTooManyPages, rng, maxPages, size)
		}
		invs, _, err := p.client.ListInvoices(ctx, excellentbooks.ListParams{
			Fields: ebInvoicePrepaymentFields, Sort: "TransDate", Range: rng, Limit: size, Offset: offset, Filter: filter,
		})
		if err != nil {
			return nil, err
		}
		fresh := invs
		if page > 0 {
			if len(invs) == 0 || invs[0].SerNr != anchor {
				got := "nothing"
				if len(invs) > 0 {
					got = fmt.Sprintf("%q", invs[0].SerNr)
				}
				return nil, fmt.Errorf("%w: IVVc range %s page %d at offset %d starts with %s, expected invoice %q",
					ErrPageShifted, rng, page+1, offset, got, anchor)
			}
			fresh = invs[1:]
		}
		for _, inv := range fresh {
			if i, ok := seen[inv.SerNr]; ok && inv.SerNr != "" {
				out[i] = inv // the later copy wins
				continue
			}
			seen[inv.SerNr] = len(out)
			out = append(out, inv)
		}
		if len(invs) < size {
			return out, nil
		}
		anchor = invs[len(invs)-1].SerNr
		if anchor == "" {
			return nil, fmt.Errorf("%w: IVVc range %s page %d ends with an invoice without SerNr; pages cannot be joined", ErrPageShifted, rng, page+1)
		}
		offset += size - 1
	}
}
