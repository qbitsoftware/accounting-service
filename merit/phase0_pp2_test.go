package merit

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestPhase0PrePayment2 tests the CORRECTED prepayment route from the docs:
// v2/Banks/{bankId}/PrePayments/ForCustomer/{customerId}
func TestPhase0PrePayment2(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	path := fmt.Sprintf("v2/Banks/%s/PrePayments/ForCustomer/%s", phase0SwedBankID, phase0CustomerID)
	for _, d := range []string{"2027-01-15", "2026-06-03", "2025-06-02"} {
		body := fmt.Sprintf(`{"Description":"klubio phase0 prepayment","DocumentNumber":"KLUBIO-PP","CurrencyCode":"EUR","DocumentDate":%q,"Amount":100.00}`, d)
		st, hdr, short := c.rawReq(ctx, "POST", path, body)
		ct := ""
		if hdr != nil { ct = hdr.Get("Content-Type") }
		t.Logf("PREPAYMENT ForCustomer @%s -> status=%d ctype=%q body=%q", d, st, ct, short)
	}
}
