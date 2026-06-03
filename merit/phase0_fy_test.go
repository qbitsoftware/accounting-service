package merit

import (
	"context"
	"testing"
	"time"
)

func TestPhase0FY(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for _, ep := range []string{
		"v1/getfinancialyears", "v2/getfinancialyears",
		"v1/getfiscalyears", "v1/getcompany", "v2/getcompany",
		"v1/getgeneralsettings", "v1/getsettings", "v1/getcompanyinfo",
		"v1/gettaxpayer", "v1/getaccountingperiods",
	} {
		st, resp, _ := c.rawPost(ctx, ep, "{}")
		if len(resp) > 240 { resp = resp[:240] }
		t.Logf("%-24s status=%d resp=%s", ep, st, resp)
	}
}
