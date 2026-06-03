package merit

import (
	"context"
	"testing"
	"time"
)

// TestPhase0Discover probes candidate endpoint names to find the real
// prepayment/settlement paths. 404 => not found; 400/500 => exists (bad body).
func TestPhase0Discover(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	candidates := []string{
		// income/expense payments (not invoice-tied) — possible advance home
		"v1/getincomepayments", "v2/getincomepayments",
		"v1/sendincomepayments", "v2/sendincomepayments",
		"v1/sendincomepayment", "v2/sendincomepayment",
		"v1/getexpensepayments", "v2/getexpensepayments",
		"v1/sendexpensepayments", "v2/sendexpensepayments",
		// bank statement import + payment imports
		"v1/sendbankstatement", "v2/sendbankstatement",
		"v1/importbankstatement", "v2/importbankstatement",
		"v1/getpaymentimports", "v2/getpaymentimports",
		// prepayment naming variants
		"v1/prepayment", "v2/prepayment", "v1/prepayments", "v2/prepayments",
		"v1/sendsettlement", "v2/sendsettlement", // settlement (control: v2 exists)
	}
	for _, ep := range candidates {
		status, resp, err := c.rawPost(ctx, ep, "{}")
		if err != nil {
			t.Logf("%-22s transport error: %v", ep, err)
			continue
		}
		short := resp
		if len(short) > 160 {
			short = short[:160]
		}
		t.Logf("%-22s status=%d body=%s", ep, status, short)
	}
}
