package accounting

import (
	"testing"
	"time"

	"github.com/qbitsoftware/accounting-service/smartaccounts"
	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestSADeriveStatus(t *testing.T) {
	tests := []struct {
		name        string
		total       string
		outstanding string
		wantStatus  InvoiceStatus
		wantPaid    bool
	}{
		{"fully paid", "100", "0", InvoiceStatusPaid, true},
		{"unpaid", "100", "100", InvoiceStatusUnpaid, false},
		{"partial", "100", "40", InvoiceStatusPartial, false},
		{"overpaid", "100", "-5", InvoiceStatusPaid, true},
		// Credit notes carry a negative total; outstanding runs from total
		// (unsettled) toward zero (fully offset) and must NOT read as paid.
		{"credit note unsettled", "-120", "-120", InvoiceStatusUnpaid, false},
		{"credit note partial", "-120", "-50", InvoiceStatusPartial, false},
		{"credit note offset", "-120", "0", InvoiceStatusPaid, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, paid := deriveSAStatus(d(tt.total), d(tt.outstanding))
			if status != tt.wantStatus || paid != tt.wantPaid {
				t.Errorf("got (%s, %v), want (%s, %v)", status, paid, tt.wantStatus, tt.wantPaid)
			}
		})
	}
}

func TestSAPaidAmount(t *testing.T) {
	cases := []struct{ name, total, outstanding, want string }{
		{"unpaid", "100", "100", "0"},
		{"partial", "100", "40", "60"},
		{"paid", "100", "0", "100"},
		{"credit note unsettled", "-120", "-120", "0"},
		{"credit note partial", "-120", "-50", "70"},
		{"credit note offset", "-120", "0", "120"}, // must be positive, not -120
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := saPaidAmount(d(c.total), d(c.outstanding))
			if !got.Equal(d(c.want)) {
				t.Errorf("saPaidAmount(%s,%s) = %s, want %s", c.total, c.outstanding, got, c.want)
			}
		})
	}
}

func TestMapSAInvoice(t *testing.T) {
	item := smartaccounts.InvoiceItem{
		ID:                "55",
		InvoiceNumber:     "2025-7",
		ClientID:          "9",
		Client:            &smartaccounts.PartnerRef{ID: "9", Name: "Acme OÜ"},
		Date:              "03.02.2025",
		DueDate:           "17.02.2025",
		Currency:          "EUR",
		TotalAmount:       d("120"),
		VatAmount:         d("20"),
		OutstandingAmount: d("120"),
		ReferenceNumber:   "1234",
		Rows: []smartaccounts.InvoiceRow{
			{Description: "Membership", Quantity: d("1"), Price: d("100"), VatPc: "20", Sum: d("120")},
		},
	}
	inv := mapSAInvoice(item)

	if inv.ID != "55" || inv.Number != "2025-7" {
		t.Errorf("id/number mismatch: %+v", inv)
	}
	if inv.CustomerName != "Acme OÜ" {
		t.Errorf("customer name should come from nested client: %q", inv.CustomerName)
	}
	if !inv.DocDate.Equal(time.Date(2025, 2, 3, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("dd.MM.yyyy date should parse: %v", inv.DocDate)
	}
	if inv.Status != InvoiceStatusUnpaid || inv.Paid {
		t.Errorf("outstanding==total should be unpaid: %s paid=%v", inv.Status, inv.Paid)
	}
	if !inv.PaidAmount.Equal(d("0")) {
		t.Errorf("paid amount should be total-outstanding=0, got %s", inv.PaidAmount)
	}
	if len(inv.Lines) != 1 || inv.Lines[0].TaxID != "20" {
		t.Errorf("row should map vatPc to TaxID: %+v", inv.Lines)
	}
}

func TestMapSAPayment(t *testing.T) {
	item := smartaccounts.PaymentItem{
		ID:          "p1",
		Number:      "PAY-1",
		Date:        "10.03.2025",
		PartnerType: smartaccounts.PartnerClient,
		Client:      &smartaccounts.PartnerRef{ID: "9", Name: "Acme OÜ"},
		AccountType: smartaccounts.AccountBank,
		AccountName: "Swedbank EUR",
		Currency:    "EUR",
		Amount:      d("120"),
		Rows: []smartaccounts.PaymentRow{
			{Type: smartaccounts.RowClientInvoice, ID: "55", Amount: d("120")},
			{Type: "PREPAYMENT_PAYMENT", Amount: d("0")},
		},
	}
	pay := mapSAPayment(item)

	if pay.Direction != PaymentDirectionCustomer {
		t.Errorf("CLIENT partner should map to customer direction, got %s", pay.Direction)
	}
	if pay.CounterPartName != "Acme OÜ" {
		t.Errorf("counterpart name mismatch: %q", pay.CounterPartName)
	}
	if pay.ExternalBankName != "Swedbank EUR" {
		t.Errorf("bank name should be account name, got %q", pay.ExternalBankName)
	}
	if len(pay.InvoiceLinks) != 1 || pay.InvoiceLinks[0].InvoiceID != "55" {
		t.Errorf("only invoice rows should become links: %+v", pay.InvoiceLinks)
	}
}

func TestBuildSARowsMapsVatPc(t *testing.T) {
	rows := buildSARows([]CreateInvoiceLineInput{
		{Code: "00010", Description: "X", Quantity: d("2"), UnitPrice: d("5"), TaxID: "20", AccountCode: "3000"},
	})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].VatPc != "20" {
		t.Errorf("TaxID should map to vatPc, got %q", rows[0].VatPc)
	}
	if rows[0].AccountSales != "3000" {
		t.Errorf("AccountCode should map to accountSales, got %q", rows[0].AccountSales)
	}
}

func TestSADateRoundTrip(t *testing.T) {
	when := time.Date(2025, 12, 9, 0, 0, 0, 0, time.UTC)
	if got := saFormatDate(when); got != "09.12.2025" {
		t.Errorf("saFormatDate = %q, want 09.12.2025", got)
	}
	if got := saParseDate("09.12.2025"); !got.Equal(when) {
		t.Errorf("saParseDate round-trip failed: %v", got)
	}
	if saFormatDate(time.Time{}) != "" {
		t.Error("zero time should format to empty string")
	}
}

func TestItemTypeMapping(t *testing.T) {
	cases := map[ItemType]string{
		ItemTypeStock:   smartaccounts.ArticleWarehouse,
		ItemTypeService: smartaccounts.ArticleService,
		ItemTypeItem:    smartaccounts.ArticleProduct,
	}
	for in, want := range cases {
		if got := itemTypeToSA(in); got != want {
			t.Errorf("itemTypeToSA(%s) = %q, want %q", in, got, want)
		}
		if got := itemTypeFromSA(want); got != in {
			t.Errorf("itemTypeFromSA(%q) = %s, want %s", want, got, in)
		}
	}
}

func TestSACapabilities(t *testing.T) {
	caps := ProviderCapabilities("smartaccounts")
	if !caps.SupportsIncrementalSync {
		t.Error("SmartAccounts should support incremental sync (modifydate)")
	}
	if !caps.SupportsInvoicePDF || !caps.SupportsFindInvoiceByRef {
		t.Error("SmartAccounts should support invoice PDF and find-by-ref")
	}
}
