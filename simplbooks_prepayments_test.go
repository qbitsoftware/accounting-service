package accounting

import (
	"testing"
	"time"

	"github.com/qbitsoftware/accounting-service/simplbooks"
)

func TestSimplBooksPrepaymentDescription(t *testing.T) {
	cases := []struct {
		no, comment, want string
	}{
		{"", "", "[ETTEMAKS]"},
		{"KLB-PP-7", "", "[ETTEMAKS] KLB-PP-7"},
		{"KLB-PP-7", "March advance", "[ETTEMAKS] KLB-PP-7 March advance"},
		{"", "loose cash", "[ETTEMAKS] loose cash"},
	}
	for _, c := range cases {
		if got := simplbooksPrepaymentDescription(c.no, c.comment); got != c.want {
			t.Errorf("desc(%q,%q) = %q, want %q", c.no, c.comment, got, c.want)
		}
	}
}

func TestSimplBooksPrepaymentsFromIncomings(t *testing.T) {
	inc := func(id, client int, sum, date, desc string) simplbooks.Incoming {
		return simplbooks.Incoming{
			ID:           id,
			ClientID:     client,
			IncomeSum:    decSB(sum),
			IncomeDate:   date,
			CurrencyName: "EUR",
			Description:  desc,
		}
	}
	incs := []simplbooks.Incoming{
		inc(1, 100, "50.00", "2024-03-01", "[ETTEMAKS] KLB-PP-1"),     // advance, client 100
		inc(2, 100, "20.00", "2024-03-05", "Invoice 18002 payment"),   // ordinary receipt — excluded (no tag)
		inc(3, 200, "80.00", "2024-03-10", "[ETTEMAKS] other client"), // advance, client 200
		inc(4, 100, "10.00", "2023-01-01", "[ETTEMAKS] too old"),      // advance, out of window
	}

	d := func(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }
	since, until := d("2024-01-01"), d("2024-12-31")

	// Scoped to client 100 within the window: only incoming #1.
	got := simplbooksPrepaymentsFromIncomings(incs, "100", since, until)
	if len(got) != 1 {
		t.Fatalf("got %d prepayments, want 1: %+v", len(got), got)
	}
	if got[0].Number != "1" || got[0].CustomerCode != "100" {
		t.Errorf("wrong prepayment: %+v", got[0])
	}
	if got[0].Amount.String() != "50" || got[0].Remaining.String() != "50" {
		t.Errorf("amount/remaining = %s/%s, want 50/50", got[0].Amount, got[0].Remaining)
	}

	// No customer scope, no window: both tagged+in-nothing-excluded advances,
	// i.e. #1, #3, #4 (all tagged; window open) — ordinary receipt #2 excluded.
	all := simplbooksPrepaymentsFromIncomings(incs, "", time.Time{}, time.Time{})
	if len(all) != 3 {
		t.Fatalf("got %d prepayments, want 3: %+v", len(all), all)
	}
}
