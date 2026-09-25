package accounting

import (
	"testing"

	"github.com/qbitsoftware/accounting-service/excellentbooks"
	"github.com/shopspring/decimal"
)

// A receipt pull has to say which part of an invoice's paid amount came from an
// advance. Without it the caller cannot tell a credit application from a cash
// payment, and re-plans an application EB already holds against an advance it
// has already emptied (Tähtvere, 22.09.2026: a 90,00 online payment was booked
// as 60,00 because 30,00 of the gap was attributed to an existing credit row).
func TestExcellentReceiptInvoiceLinks_FromPrepayment(t *testing.T) {
	row := func(inv, cup, val string) excellentbooks.ReceiptRow {
		return excellentbooks.ReceiptRow{InvoiceNr: inv, CUPNr: cup, RecVal: val}
	}
	d := decimal.RequireFromString

	tests := []struct {
		name string
		rows []excellentbooks.ReceiptRow
		want []PaymentInvoiceLink
	}{
		{
			name: "cash receipt",
			rows: []excellentbooks.ReceiptRow{row("1611", "", "45")},
			want: []PaymentInvoiceLink{{InvoiceID: "1611", InvoiceNo: "1611", Amount: d("45"), FromPrepayment: d("0")}},
		},
		{
			name: "prepayment applied to an invoice",
			rows: []excellentbooks.ReceiptRow{row("1611", "", "75"), row("", "90000006", "-75")},
			want: []PaymentInvoiceLink{{InvoiceID: "1611", InvoiceNo: "1611", Amount: d("75"), FromPrepayment: d("75")}},
		},
		{
			name: "paid money freed into a prepayment",
			rows: []excellentbooks.ReceiptRow{row("1396", "", "-120"), row("", "90000000", "120")},
			want: []PaymentInvoiceLink{{InvoiceID: "1396", InvoiceNo: "1396", Amount: d("-120"), FromPrepayment: d("-120")}},
		},
		{
			// Money arriving with an overpayment parked beside it: both rows are
			// cash, and the advance row must not be read as funding the invoice.
			name: "cash with the excess parked as an advance",
			rows: []excellentbooks.ReceiptRow{row("1611", "", "90"), row("", "2704", "30")},
			want: []PaymentInvoiceLink{{InvoiceID: "1611", InvoiceNo: "1611", Amount: d("90"), FromPrepayment: d("0")}},
		},
		{
			name: "advance drawn across two invoices",
			rows: []excellentbooks.ReceiptRow{row("1", "", "30"), row("2", "", "50"), row("", "2704", "-60")},
			want: []PaymentInvoiceLink{
				{InvoiceID: "1", InvoiceNo: "1", Amount: d("30"), FromPrepayment: d("30")},
				{InvoiceID: "2", InvoiceNo: "2", Amount: d("50"), FromPrepayment: d("30")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := excellentReceiptInvoiceLinks(tt.rows)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d links, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range got {
				g, w := got[i], tt.want[i]
				if g.InvoiceID != w.InvoiceID || !g.Amount.Equal(w.Amount) || !g.FromPrepayment.Equal(w.FromPrepayment) {
					t.Errorf("link %d = {%s %s from_prepayment=%s}, want {%s %s from_prepayment=%s}",
						i, g.InvoiceID, g.Amount, g.FromPrepayment, w.InvoiceID, w.Amount, w.FromPrepayment)
				}
			}
		})
	}
}
