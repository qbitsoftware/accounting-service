package accounting

import (
	"testing"

	"github.com/qbitsoftware/accounting-service/merit"
	"github.com/shopspring/decimal"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestMeritAdvancesFromDebts(t *testing.T) {
	items := []merit.CustomerDebtItem{
		// a real ettemaks (advance) — included
		{DocType: "BA", DocNo: "KLB-PP-0", DocID: "doc-1", PartnerID: "cust-1",
			TotalAmount: dec("-20.00"), UnPaidAmount: dec("-20.00"), CurrencyCode: "EUR", DocDate: "2026-06-03T00:00:00"},
		// an accountant advance, partially used — Remaining reflects the net
		{DocType: "BA", DocNo: "ACCT-1", DocID: "doc-2", PartnerID: "cust-1",
			TotalAmount: dec("-30.00"), UnPaidAmount: dec("-18.00"), CurrencyCode: "EUR", DocDate: "2026-06-03T00:00:00"},
		// a sales invoice (positive MA) — excluded
		{DocType: "MA", DocNo: "INV1", DocID: "doc-3", PartnerID: "cust-1",
			TotalAmount: dec("45.00"), UnPaidAmount: dec("45.00"), CurrencyCode: "EUR"},
		// a CREDIT NOTE (negative MA) — MUST be excluded (else double-count)
		{DocType: "MA", DocNo: "KRINV1", DocID: "doc-4", PartnerID: "cust-1",
			TotalAmount: dec("-45.00"), UnPaidAmount: dec("-45.00"), CurrencyCode: "EUR"},
		// a fully-applied advance (zero) — excluded by the negative check
		{DocType: "BA", DocNo: "KLB-PP-9", DocID: "doc-5", PartnerID: "cust-1",
			TotalAmount: dec("-10.00"), UnPaidAmount: dec("0.00"), CurrencyCode: "EUR"},
	}

	got := meritAdvancesFromDebts(items)

	if len(got) != 2 {
		t.Fatalf("expected 2 advances (the BA negatives), got %d: %+v", len(got), got)
	}
	if got[0].Number != "KLB-PP-0" || !got[0].Remaining.Equal(dec("20.00")) || got[0].DocID != "doc-1" {
		t.Errorf("advance[0] wrong: %+v", got[0])
	}
	if got[1].Number != "ACCT-1" || !got[1].Remaining.Equal(dec("18.00")) {
		t.Errorf("advance[1] remaining should be the net 18.00, got %s", got[1].Remaining)
	}
	for _, a := range got {
		if a.CustomerCode != "cust-1" {
			t.Errorf("customer code not mapped: %+v", a)
		}
		if a.Remaining.IsNegative() {
			t.Errorf("remaining must be positive (credit), got %s", a.Remaining)
		}
	}
}

func TestMeritAdvancesFromDebts_excludesCreditNotes(t *testing.T) {
	// A customer whose only negative line is a credit note must yield no advances.
	items := []merit.CustomerDebtItem{
		{DocType: "MA", DocNo: "KRINV1", DocID: "d", PartnerID: "c",
			TotalAmount: dec("-100.00"), UnPaidAmount: dec("-100.00"), CurrencyCode: "EUR"},
	}
	if got := meritAdvancesFromDebts(items); len(got) != 0 {
		t.Fatalf("credit note must not be treated as an advance, got %d: %+v", len(got), got)
	}
}
