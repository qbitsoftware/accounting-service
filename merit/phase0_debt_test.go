package merit

import (
	"context"
	"testing"
	"time"
)

func TestPhase0Debt(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// customer debt report for our test customer at 2026-06-03
	_, resp, _ := c.rawPost(ctx, "v1/getcustdebtrep", `{"CustId":"b6167698-26f1-4aa5-abac-976cd828e297","DebtDate":"20260603"}`)
	t.Logf("CUSTDEBTREP (by CustId):\n%s", resp)
	// also all customers
	_, resp2, _ := c.rawPost(ctx, "v1/getcustdebtrep", `{"DebtDate":"20260603"}`)
	if len(resp2) > 1500 { resp2 = resp2[:1500] }
	t.Logf("CUSTDEBTREP (all):\n%s", resp2)
}
