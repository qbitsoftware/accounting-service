package merit

import (
	"context"
	"os"
	"testing"
	"time"
)

// Creates a prepayment for an arbitrary customer (simulating the accountant).
//   MERIT_CUST_ID=<guid> MERIT_PP_NO=ACCT-TEST MERIT_PP_AMOUNT=30 go test ./merit -run TestMeritMkAdvance -count=1 -v
func TestMeritMkAdvance(t *testing.T) {
	c := phase0Client(t)
	cust := os.Getenv("MERIT_CUST_ID")
	no := os.Getenv("MERIT_PP_NO")
	amt := os.Getenv("MERIT_PP_AMOUNT")
	if cust == "" || no == "" || amt == "" {
		t.Skip("need MERIT_CUST_ID / MERIT_PP_NO / MERIT_PP_AMOUNT")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	path := "v2/Banks/" + phase0SwedBankID + "/PrePayments/ForCustomer/" + cust
	body := `{"Description":"accountant manual advance (test)","DocumentNumber":"` + no + `","CurrencyCode":"EUR","DocumentDate":"2026-06-03","Amount":` + amt + `}`
	st, resp, err := c.rawPost(ctx, path, body)
	t.Logf("CREATE ADVANCE %s for %s -> status=%d resp=%s err=%v", no, cust, st, resp, err)
}
