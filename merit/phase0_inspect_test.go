package merit

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestMeritInspect dumps a customer's full Merit state — advances (ettemaks),
// payments, and open invoices — for verifying klubio actions end-to-end.
//
//	MERIT_API_ID=... MERIT_API_KEY=... MERIT_CUST_ID=<guid> \
//	  go test ./merit -run TestMeritInspect -count=1 -v
//
// Provide MERIT_CUST_ID (customer GUID) or MERIT_CUST_NAME (filters debt report).
func TestMeritInspect(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	custID := os.Getenv("MERIT_CUST_ID")
	custName := os.Getenv("MERIT_CUST_NAME")
	debtDate := os.Getenv("MERIT_DEBT_DATE") // yyyyMMdd; empty = today on Merit
	if debtDate == "" {
		debtDate = time.Now().UTC().Format("20060102")
	}

	// 1) Customer debt report — advances are negative (DocType BA), invoices positive (MA).
	body := `{"DebtDate":"` + debtDate + `"}`
	if custID != "" {
		body = `{"CustId":"` + custID + `","DebtDate":"` + debtDate + `"}`
	} else if custName != "" {
		body = `{"CustName":"` + custName + `","DebtDate":"` + debtDate + `"}`
	}
	_, debt, _ := c.rawPost(ctx, "v1/getcustdebtrep", body)
	t.Logf("=== CUSTOMER DEBT (advances = negative BA lines; invoices = MA) ===\n%s", debt)

	// 2) Recent payments (last 90d) — prepayments & invoice payments both appear here.
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -89)
	pays, err := c.ListPayments(ctx, ListPaymentsParams{PeriodStart: start.Format("20060102"), PeriodEnd: end.Format("20060102")})
	if err != nil {
		t.Logf("getpayments error: %v", err)
	} else {
		t.Logf("=== PAYMENTS (last 90d): %d ===", len(pays))
		for _, p := range pays {
			if custID != "" && p.CounterPartID != "" && p.CounterPartID != custID {
				continue
			}
			t.Logf("  DocNo=%-16s Dir=%d Amount=%s Bank=%s Date=%s Links=%d",
				p.DocumentNo, p.Direction, p.Amount.String(), p.BankName, p.DocumentDate, len(p.PaymAPIDetails))
		}
	}
}
