package merit

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// Phase 0 live verification harness. Read-only probe. Runs only when
// MERIT_API_ID / MERIT_API_KEY are set in the environment.
//
//	MERIT_API_ID=... MERIT_API_KEY=... go test ./merit -run TestPhase0Probe -v
func phase0Client(t *testing.T) *Client {
	t.Helper()
	id := os.Getenv("MERIT_API_ID")
	key := os.Getenv("MERIT_API_KEY")
	if id == "" || key == "" {
		t.Skip("MERIT_API_ID / MERIT_API_KEY not set")
	}
	url := os.Getenv("MERIT_API_URL")
	if url == "" {
		url = EstoniaURL
	}
	return New(Config{APIURL: url, APIID: id, APIKey: key})
}

func dump(t *testing.T, label string, v any) {
	t.Helper()
	b, _ := json.MarshalIndent(v, "", "  ")
	t.Logf("%s:\n%s", label, string(b))
}

func TestPhase0Probe(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	banks, err := c.ListBanks(ctx)
	if err != nil {
		t.Fatalf("ListBanks: %v", err)
	}
	dump(t, "BANKS", banks)

	custs, err := c.ListCustomers(ctx, ListCustomersParams{})
	if err != nil {
		t.Fatalf("ListCustomers: %v", err)
	}
	t.Logf("CUSTOMER COUNT: %d", len(custs))
	if len(custs) > 5 {
		custs = custs[:5]
	}
	dump(t, "CUSTOMERS (first 5)", custs)

	end := time.Now().UTC()
	start := end.AddDate(0, 0, -89) // getpayments max window is 3 months
	pays, err := c.ListPayments(ctx, ListPaymentsParams{
		PeriodStart: start.Format("20060102"),
		PeriodEnd:   end.Format("20060102"),
	})
	if err != nil {
		t.Fatalf("ListPayments: %v", err)
	}
	t.Logf("PAYMENT COUNT (last 90d): %d", len(pays))
	if len(pays) > 8 {
		pays = pays[:8]
	}
	dump(t, "PAYMENTS (first 8)", pays)

	invStart := end.AddDate(0, 0, -120)
	invs, err := c.ListInvoices(ctx, ListInvoicesParams{
		PeriodStart: invStart.Format("20060102"),
		PeriodEnd:   end.Format("20060102"),
	})
	if err != nil {
		t.Fatalf("ListInvoices: %v", err)
	}
	t.Logf("INVOICE COUNT (last 120d): %d", len(invs))
	if len(invs) > 5 {
		invs = invs[:5]
	}
	dump(t, "INVOICES (first 5)", invs)
}
