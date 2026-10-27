package accounting

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/qbitsoftware/accounting-service/merit"
	"github.com/shopspring/decimal"
)

// One payment exactly as v2/getpayments answered it (Merit test company,
// 27.10.2026; names changed). Every field the payment list is read for was
// missing through the old field names — the id, the date and the links — so a
// caller could never tell which invoice a payment paid or when.
const meritGetPaymentsLive = `[{"PHId":"a2b21b52-d919-49b3-0e43-08df1d85d527","BankName":"Kassa","CounterPartType":2,"CounterPartName":"Mari Maasikas","CurrencyCode":"EUR","CurrencyRate":1.0000000,"DocumentDate":"2026-09-29T00:00:00","DocumentNo":"35","Direction":1,"Amount":30.00,"Details":[{"PaymId":"a2b21b52-d919-49b3-0e43-08df1d85d527","DocId":"95055ca8-2185-4423-af35-08df1d85d099","DocNo":"MT000195","DocAmount":75.00,"PaidAmount":30.00,"CurrencyCode":"EUR","CurrencyRate":1.0000000}],"ChangedDate":"2026-09-29T16:36:50.383"}]`

func TestMeritPaymentListReadsWhatMeritSends(t *testing.T) {
	var items []merit.PaymentListItem
	if err := json.Unmarshal([]byte(meritGetPaymentsLive), &items); err != nil {
		t.Fatal(err)
	}
	got := mapPaymentListItems(items)
	if len(got) != 1 {
		t.Fatalf("got %d payments, want 1", len(got))
	}
	p := got[0]
	if p.ID != "a2b21b52-d919-49b3-0e43-08df1d85d527" {
		t.Errorf("ID = %q, want the PHId", p.ID)
	}
	if want := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC); !p.DocumentDate.Equal(want) {
		t.Errorf("DocumentDate = %v, want %v", p.DocumentDate, want)
	}
	if p.ExternalBankName != "Kassa" {
		t.Errorf("ExternalBankName = %q, want Kassa", p.ExternalBankName)
	}
	if len(p.InvoiceLinks) != 1 {
		t.Fatalf("got %d invoice links, want 1 (from Details)", len(p.InvoiceLinks))
	}
	l := p.InvoiceLinks[0]
	if l.InvoiceNo != "MT000195" || l.InvoiceID != "95055ca8-2185-4423-af35-08df1d85d099" || !l.Amount.Equal(decimal.NewFromInt(30)) {
		t.Errorf("link = %+v, want MT000195 / its DocId / 30", l)
	}
}

// The field names this struct was first written with still map, so nothing
// that encodes the old shape (a test double, a recorded fixture) changes.
func TestMeritPaymentListKeepsTheOldFieldNames(t *testing.T) {
	got := mapPaymentListItem(merit.PaymentListItem{
		PIHId:          "old-id",
		DocumentDate:   "20260911",
		PaymAPIDetails: []merit.PaymAPIDetail{{DocNo: "INV1", DocID: "doc-1", PaidAmount: decimal.NewFromInt(85)}},
	})
	if got.ID != "old-id" {
		t.Errorf("ID = %q, want old-id", got.ID)
	}
	if want := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC); !got.DocumentDate.Equal(want) {
		t.Errorf("DocumentDate = %v, want %v", got.DocumentDate, want)
	}
	if len(got.InvoiceLinks) != 1 || got.InvoiceLinks[0].InvoiceNo != "INV1" {
		t.Errorf("links = %+v, want INV1", got.InvoiceLinks)
	}
}

// getinvoice dates a receipt "2026-09-27T00:00:00Z" (Merit live, 28.09.2026).
// Read as yyyymmdd it was zero, and every caller stamped the day it asked.
func TestMeritInvoiceReceiptsCarryTheirDate(t *testing.T) {
	inv := mapInvoiceDetail(&merit.InvoiceDetail{
		Payments: []merit.PaymentInfo{{PaymDate: "2026-09-27T00:00:00Z", Amount: decimal.NewFromInt(160), PaymentID: "r-1"}},
	})
	if len(inv.Payments) != 1 {
		t.Fatalf("got %d receipts, want 1", len(inv.Payments))
	}
	if want := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC); !inv.Payments[0].Date.Equal(want) {
		t.Errorf("receipt date = %v, want %v", inv.Payments[0].Date, want)
	}
}

func TestParseMeritAnyDate(t *testing.T) {
	want := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	for _, in := range []string{"20260927", "2026-09-27T00:00:00", "2026-09-27T00:00:00Z", "2026-09-27"} {
		if got := parseMeritAnyDate(in); !got.Equal(want) {
			t.Errorf("parseMeritAnyDate(%q) = %v, want %v", in, got, want)
		}
	}
	if got := parseMeritAnyDate("2026-09-27T16:36:50.383"); got.Format("2006-01-02") != "2026-09-27" {
		t.Errorf("fractional seconds: got %v", got)
	}
	for _, in := range []string{"", "not a date"} {
		if got := parseMeritAnyDate(in); !got.IsZero() {
			t.Errorf("parseMeritAnyDate(%q) = %v, want zero", in, got)
		}
	}
}
