package merit

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestPhase0Year(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// find years with invoice data
	for _, yr := range []string{"2022", "2023", "2024", "2025", "2026"} {
		st, resp, _ := c.rawPost(ctx, "v2/getinvoices", fmt.Sprintf(`{"PeriodStart":"%s0101","PeriodEnd":"%s1231"}`, yr, yr))
		short := resp
		if len(short) > 100 { short = short[:100] }
		t.Logf("invoices %s status=%d resp=%s", yr, st, short)
	}
	// probe which DocDate the company accepts by trying a throwaway date-only validation via getbalancerep (no write)
	_, br, _ := c.rawPost(ctx, "v1/getbalancerep", fmt.Sprintf(`{"Date":"%s"}`, time.Now().UTC().Format("20060102")))
	if len(br) > 200 { br = br[:200] }
	t.Logf("balancerep today: %s", br)
}
