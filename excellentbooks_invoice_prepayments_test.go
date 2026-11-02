package accounting

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/qbitsoftware/accounting-service/excellentbooks"
)

// --- fixtures ---

// ebIVRec renders one IVVc record as the prepayment read gets it back (only
// the fields it asks for); rows are ebIVRow outputs.
func ebIVRec(serNr, date, cust, okFlag, invType string, rows ...string) string {
	return `{"@sequence":"1","@url":"/api/1/IVVc/` + serNr + `","SerNr":"` + serNr + `","TransDate":"` + date + `","InvDate":"` + date +
		`","CustCode":"` + cust + `","OKFlag":"` + okFlag + `","Invalid":"0","InvType":"` + invType + `","CurncyCode":"EUR","rows":[` +
		strings.Join(rows, ",") + `]}`
}

func ebIVRow(n, stp, cupnr, sum string) string {
	return `{"@rownumber":"` + n + `","stp":"` + stp + `","CUPNr":"` + cupnr + `","Sum":"` + sum + `"}`
}

func ebIVPage(recs ...string) string {
	return `{"data":{"@register":"IVVc","@sequence":"114335","IVVc":[` + strings.Join(recs, ",") + `]}}`
}

const ebIVQuery = `fields=SerNr%2CTransDate%2CInvDate%2CCustCode%2COKFlag%2CInvalid%2CInvType%2CCurncyCode%2Cstp%2CCUPNr%2CSum`

// renderDrawn prints records compactly: id, flags, date, customer and rows.
func renderDrawn(rs []Receipt) string {
	var b strings.Builder
	for _, r := range rs {
		b.WriteString(r.ID)
		if r.InvoicePrepayment {
			b.WriteString(" invoice-prepayment")
		}
		if r.Draft {
			b.WriteString(" draft")
		}
		if r.Void {
			b.WriteString(" void")
		}
		b.WriteString(" " + r.Date.Format("2006-01-02") + " " + r.CustomerID + " amount=" + r.Amount.String())
		for _, row := range r.Rows {
			b.WriteString(" [" + row.Type + " " + row.DocID + " " + row.Amount.String() + "]")
		}
		if len(r.AdvanceSourceIDs) > 0 {
			b.WriteString(" draws=" + strings.Join(r.AdvanceSourceIDs, ","))
		}
		if !r.AdvanceAmount.IsZero() {
			b.WriteString(" parks=" + r.AdvanceAmount.String())
		}
		if r.DecodeError != "" {
			b.WriteString(" error=" + r.DecodeError)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// --- the mapping ---

// The shape Excellent's own demo invoices carry (210009: 500 of 578 drawn, the
// rest open), the one JJ-Street's accountant uses (221969: the whole 65
// drawn), and the cases that must not read as money.
func TestEBInvoicePrepaymentsMapping(t *testing.T) {
	invoice := func(serNr, okFlag, invalid, invType string, rows ...excellentbooks.InvoiceRow) excellentbooks.Invoice {
		return excellentbooks.Invoice{SerNr: serNr, TransDate: "2026-10-08", InvDate: "2026-10-08", CustCode: "K82844",
			OKFlag: okFlag, Invalid: excellentbooks.FlexString(invalid), InvType: invType, CurncyCode: "EUR", Rows: rows}
	}
	item := excellentbooks.InvoiceRow{Stp: "1", Sum: "65.00", ArtCode: "KOOLITUS"}
	prepay := func(cupnr, sum string) excellentbooks.InvoiceRow {
		return excellentbooks.InvoiceRow{Stp: "6", CUPNr: cupnr, Sum: sum, SalesAcc: "2210", Spec: "Ettemaks"}
	}
	for name, tc := range map[string]struct {
		inv  excellentbooks.Invoice
		want string
	}{
		"whole invoice drawn": {invoice("221969", "1", "0", "1", prepay("56", "65.00"), item),
			"IV221969-56 invoice-prepayment 2026-10-08 K82844 amount=0 [CLIENT_INVOICE 221969 65] [PREPAYMENT_PAYMENT 56 -65] draws=56\n"},
		"one prepayment over two rows, another beside it": {invoice("221970", "1", "0", "1", prepay("56", "30"), prepay("57", "10.5"), prepay("56", "5"), item),
			"IV221970-56 invoice-prepayment 2026-10-08 K82844 amount=0 [CLIENT_INVOICE 221970 35] [PREPAYMENT_PAYMENT 56 -35] draws=56\n" +
				"IV221970-57 invoice-prepayment 2026-10-08 K82844 amount=0 [CLIENT_INVOICE 221970 10.5] [PREPAYMENT_PAYMENT 57 -10.5] draws=57\n"},
		"unconfirmed invoice": {invoice("221971", "0", "0", "1", prepay("56", "65")),
			"IV221971-56 invoice-prepayment draft 2026-10-08 K82844 amount=0 [CLIENT_INVOICE 221971 65] [PREPAYMENT_PAYMENT 56 -65] draws=56\n"},
		"invalidated invoice": {invoice("221972", "1", "1", "1", prepay("56", "65")),
			"IV221972-56 invoice-prepayment void 2026-10-08 K82844 amount=0 [CLIENT_INVOICE 221972 65] [PREPAYMENT_PAYMENT 56 -65] draws=56\n"},
		"negative row frees into the prepayment": {invoice("221973", "1", "0", "1", prepay("56", "-20")),
			"IV221973-56 invoice-prepayment 2026-10-08 K82844 amount=0 [CLIENT_INVOICE 221973 -20] [PREPAYMENT_PAYMENT 56 20] parks=20\n"},
		"no prepayment number": {invoice("221974", "1", "0", "1", prepay("", "65")),
			"IV221974- invoice-prepayment 2026-10-08 K82844 amount=0 [CLIENT_INVOICE 221974 65] [PREPAYMENT_PAYMENT  -65] error=prepayment row 0 names no prepayment (CUPNr)\n"},
		"unreadable sum": {invoice("221975", "1", "0", "1", prepay("56", "x")),
			"IV221975-56 invoice-prepayment 2026-10-08 K82844 amount=0 [CLIENT_INVOICE 221975 0] [PREPAYMENT_PAYMENT 56 0] error=prepayment row 0: unreadable Sum \"x\"\n"},
		"credit note":        {invoice("221976", "1", "0", "3", prepay("56", "65")), ""},
		"no prepayment rows": {invoice("221977", "1", "0", "1", item), ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := renderDrawn(ebInvoicePrepayments(tc.inv)); got != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

// Every record is told apart from a receipt by its id: an IPVc SerNr is a
// number, and the id must stay inside the 50 characters klubio stores.
func TestEBInvoicePrepaymentIDIsNeverAReceiptNumber(t *testing.T) {
	id := EBInvoicePrepaymentID("2219690000", "9999999999")
	if _, err := strconv.ParseUint(id, 10, 64); err == nil {
		t.Fatalf("id %q parses as a number", id)
	}
	if len(id) > 50 {
		t.Fatalf("id %q is longer than 50 characters", id)
	}
}

// --- ListPaymentChanges ---

func TestEBListPaymentChangesWithInvoicePrepayments(t *testing.T) {
	p, f := newEBFake(t, func(n int, w ebWire) (int, string) {
		if strings.HasPrefix(w.Path, "IVVc") {
			return 200, ebIVPage(
				ebIVRec("221969", "2026-10-08", "K82844", "1", "1", ebIVRow("0", "6", "56", "65.00"), ebIVRow("1", "1", "", "65.00")),
				ebIVRec("221970", "2026-10-08", "K1", "1", "1", ebIVRow("0", "1", "", "40.00")),
			)
		}
		return 200, ebPage("5120", ebRec("1978", "2026-10-03", "1", "1", `{"stp":"1","CUPNr":"56","CustCode":"K82844","RecVal":"65.00"}`))
	})
	got, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{Since: ebDay(2026, 9, 1), WithInvoicePrepayments: true})
	if err != nil {
		t.Fatalf("ListPaymentChanges: %v", err)
	}
	assertEBWire(t, f,
		`GET IPVc?limit=500&range=2026-09-01%3A&sort=TransDate`,
		`GET IVVc?`+ebIVQuery+`&limit=500&range=2026-09-01%3A&sort=TransDate`,
	)
	if ids := receiptIDs(got.Receipts); ids != "1978,IV221969-56" {
		t.Fatalf("receipts = %s, want the receipt then the invoice's prepayment", ids)
	}
	if len(got.Payments) != 1 || got.Sequence != "5120" {
		t.Fatalf("payments %d, seq %q: Payments stays the receipts' projection and the cursor the receipt window's", len(got.Payments), got.Sequence)
	}
}

// The feed (SinceSequence) never reads invoices: the flag belongs to the date
// window.
func TestEBListPaymentChangesFeedIgnoresInvoicePrepayments(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) {
		return 200, `{"data":{"@register":"IPVc","@sequence":"9000","IPVc":[]}}`
	})
	if _, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{SinceSequence: "8000", WithInvoicePrepayments: true}); err != nil {
		t.Fatalf("ListPaymentChanges: %v", err)
	}
	for _, w := range f.wire() {
		if strings.HasPrefix(w.Path, "IVVc") {
			t.Fatalf("the feed read invoices: %s", w)
		}
	}
}

// Usable or nothing: an invoice read that fails fails the whole call, never
// a receipt list that silently lacks the invoices' prepayments.
func TestEBListPaymentChangesInvoiceReadFailureFails(t *testing.T) {
	p, _ := newEBFake(t, func(n int, w ebWire) (int, string) {
		if strings.HasPrefix(w.Path, "IVVc") {
			return 500, "boom"
		}
		return 200, ebEmptyReceipts
	})
	got, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{Since: ebDay(2026, 9, 1), WithInvoicePrepayments: true})
	if err == nil || got != nil {
		t.Fatalf("ListPaymentChanges = %v, %v; want an error and nothing", got, err)
	}
}

// The invoice read pages like the receipt read: each later page starts on the
// invoice that ended the previous one, and a list that moved is caught.
func TestEBInvoicePrepaymentReadPaginates(t *testing.T) {
	var invs []string
	for i := 0; i < 7; i++ {
		ser := strconv.Itoa(221960 + i)
		invs = append(invs, ebIVRec(ser, "2026-10-08", "K1", "1", "1", ebIVRow("0", "6", "5"+strconv.Itoa(i), "10")))
	}
	p, f := newEBFake(t, func(n int, w ebWire) (int, string) {
		if !strings.HasPrefix(w.Path, "IVVc") {
			return 200, ebEmptyReceipts
		}
		q, _ := url.ParseQuery(w.Query)
		limit, _ := strconv.Atoi(q.Get("limit"))
		offset, _ := strconv.Atoi(q.Get("offset"))
		end := offset + limit
		if end > len(invs) {
			end = len(invs)
		}
		return 200, ebIVPage(invs[offset:end]...)
	})
	p.pageSize = 3
	got, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{Since: ebDay(2026, 10, 1), WithInvoicePrepayments: true})
	if err != nil {
		t.Fatalf("ListPaymentChanges: %v", err)
	}
	if n := len(got.Receipts); n != 7 {
		t.Fatalf("%d records, want 7 (each invoice once)", n)
	}
	var ivReqs int
	for _, w := range f.wire() {
		if strings.HasPrefix(w.Path, "IVVc") {
			ivReqs++
		}
	}
	if ivReqs != 4 {
		t.Fatalf("%d invoice pages, want 4 (offsets 0, 2, 4, 6: a full page asks once more)", ivReqs)
	}

	shifting := 0
	p, _ = newEBFake(t, func(n int, w ebWire) (int, string) {
		if !strings.HasPrefix(w.Path, "IVVc") {
			return 200, ebEmptyReceipts
		}
		shifting++
		return 200, ebIVPage(invs[shifting%2 : shifting%2+3]...) // every page starts somewhere else
	})
	p.pageSize = 3
	if _, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{Since: ebDay(2026, 10, 1), WithInvoicePrepayments: true}); !errors.Is(err, ErrPageShifted) {
		t.Fatalf("err = %v, want ErrPageShifted", err)
	}
}

// --- ListInvoiceReceipts ---

func TestEBListInvoiceReceiptsWithInvoicePrepayments(t *testing.T) {
	p, f := newEBFake(t, func(n int, w ebWire) (int, string) {
		if strings.HasPrefix(w.Path, "IVVc") {
			return 200, ebIVPage(
				ebIVRec("221969", "2026-10-08", "K82844", "1", "1", ebIVRow("0", "6", "56", "65.00")),
				ebIVRec("221980", "2026-10-20", "K82844", "1", "1", ebIVRow("0", "6", "57", "10.00")),
				// The server's filter is not trusted: another customer's
				// invoice in the answer is dropped.
				ebIVRec("221990", "2026-10-21", "K1", "1", "1", ebIVRow("0", "6", "58", "10.00")),
			)
		}
		return 200, ebEmptyReceipts
	})
	all, err := p.ListInvoiceReceipts(ebCtx, ListInvoiceReceiptsInput{CustomerID: "K82844", Since: ebDay(2026, 9, 7), WithInvoicePrepayments: true})
	if err != nil {
		t.Fatalf("ListInvoiceReceipts: %v", err)
	}
	if ids := receiptIDs(all); ids != "IV221969-56,IV221980-57" {
		t.Fatalf("customer read = %s", ids)
	}
	one, err := p.ListInvoiceReceipts(ebCtx, ListInvoiceReceiptsInput{CustomerID: "K82844", InvoiceID: "221969", Since: ebDay(2026, 9, 7), WithInvoicePrepayments: true})
	if err != nil {
		t.Fatalf("ListInvoiceReceipts by invoice: %v", err)
	}
	if ids := receiptIDs(one); ids != "IV221969-56" {
		t.Fatalf("invoice read = %s", ids)
	}
	assertEBWire(t, f,
		`GET IPVc?limit=500&range=2026-09-07%3A&sort=TransDate`,
		`GET IVVc?`+ebIVQuery+`&filter.CustCode=K82844&limit=500&range=2026-09-07%3A&sort=TransDate`,
		`GET IPVc?limit=500&range=2026-09-07%3A&sort=TransDate`,
		`GET IVVc?`+ebIVQuery+`&filter.CustCode=K82844&limit=500&range=2026-09-07%3A&sort=TransDate`,
	)
}
