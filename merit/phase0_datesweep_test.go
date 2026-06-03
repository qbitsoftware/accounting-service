package merit

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestPhase0DateSweep(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	row := fmt.Sprintf(`{"Item":{"Code":%q,"Description":"x"},"Quantity":1,"Price":100.00,"TaxId":%q}`, phase0ItemCode, phase0Tax24)
	tax := fmt.Sprintf(`{"TaxId":%q,"Amount":24.00}`, phase0Tax24)
	for y := 2019; y <= 2026; y++ {
		for _, md := range []string{"01-15", "06-15", "11-15"} {
			d := fmt.Sprintf("%d-%s", y, md)
			body := fmt.Sprintf(`{"Customer":{"Id":%q},"AccountingDoc":1,"DocDate":%q,"DueDate":%q,"InvoiceNo":"KLUBIO-SWEEP","CurrencyCode":"EUR","InvoiceRow":[%s],"TaxAmount":[%s],"TotalAmount":124.00}`,
				phase0CustomerID, d, d, row, tax)
			st, resp, _ := c.rawPost(ctx, "v2/sendinvoice", body)
			msg := resp
			if len(msg) > 70 { msg = msg[:70] }
			tag := "DATE-BLOCKED"
			if st >= 200 && st < 300 { tag = ">>> ACCEPTED <<<" } else if !contains(resp, "Dokumendi kuup") { tag = "OTHER" }
			t.Logf("%s  %s  status=%d  %s", d, tag, st, msg)
			if st >= 200 && st < 300 { t.Logf("ACTIVE YEAR FOUND: %s", d); return }
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub { return true }
	}
	return false
}
