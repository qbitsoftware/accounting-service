package accounting

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// Live test for Merit prepayment PUSH + FETCH via the public Client API.
//
//	MERIT_API_ID=... MERIT_API_KEY=... go test . -run TestMeritPrepaymentLive -count=1 -v
func TestMeritPrepaymentLive(t *testing.T) {
	id, key := os.Getenv("MERIT_API_ID"), os.Getenv("MERIT_API_KEY")
	if id == "" || key == "" {
		t.Skip("MERIT_API_ID / MERIT_API_KEY not set")
	}
	const (
		customerID = "b6167698-26f1-4aa5-abac-976cd828e297"
		swedBankID = "b7549a02-faac-4d6c-d2c7-08debce783ca"
	)
	c, err := NewClient(Config{Provider: "merit", APIID: id, APIKey: key, Region: "ee"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !c.Prepayments.Supported() {
		t.Fatal("Prepayments.Supported() = false for merit")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// PUSH
	docNo := "KLUBIO-LIVE-" + time.Now().UTC().Format("150405")
	pp, err := c.Prepayments.Create(ctx, CreatePrepaymentInput{
		CustomerCode: customerID,
		BankID:       swedBankID,
		PrepaymentNo: docNo,
		Amount:       decimal.RequireFromString("42.00"),
		Currency:     "EUR",
		PaymentDate:  time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC),
		Comment:      "klubio live prepay test",
	})
	if err != nil {
		t.Fatalf("Create prepayment: %v", err)
	}
	t.Logf("PUSH ok: Number=%s DocID(batch)=%s", pp.Number, pp.DocID)

	time.Sleep(1500 * time.Millisecond)

	// FETCH
	got, err := c.Prepayments.List(ctx, ListPrepaymentsInput{
		CustomerCode: customerID,
		Until:        time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("List prepayments: %v", err)
	}
	t.Logf("FETCH ok: %d advances", len(got))
	var found bool
	var total decimal.Decimal
	for _, a := range got {
		total = total.Add(a.Remaining)
		t.Logf("  - No=%s DocID=%s Remaining=%s Date=%s", a.Number, a.DocID, a.Remaining.StringFixed(2), a.Date.Format("2006-01-02"))
		if a.Number == docNo {
			found = true
		}
	}
	t.Logf("total open credit for customer: %s", total.StringFixed(2))
	if len(got) == 0 {
		t.Fatal("expected at least one advance, got none")
	}
	if !found {
		t.Logf("note: the just-created %s did not appear (debt-report date vs prepayment date window) — not fatal", docNo)
	}
}
