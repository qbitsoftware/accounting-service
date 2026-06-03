package merit

import (
	"context"
	"time"
	"fmt"
	"testing"
)

func TestPhase0InvSweep(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	row := fmt.Sprintf(`{"Item":{"Code":%q,"Description":"x"},"Quantity":1,"Price":100.00,"TaxId":%q}`, phase0ItemCode, phase0Tax24)
	tax := fmt.Sprintf(`{"TaxId":%q,"Amount":24.00}`, phase0Tax24)
	dates := []string{
		"2025-06-03", "2025-12-01",
		"2026-05-02", "2026-06-03", "2026-07-01", "2026-09-01", "2026-12-31",
		"2027-01-15", "2027-06-15",
	}
	for _, d := range dates {
		body := fmt.Sprintf(`{"Customer":{"Id":%q},"AccountingDoc":1,"DocDate":%q,"DueDate":%q,"InvoiceNo":"KLUBIO-SW-%s","CurrencyCode":"EUR","InvoiceRow":[%s],"TaxAmount":[%s],"TotalAmount":124.00}`,
			phase0CustomerID, d, d, d, row, tax)
		st, resp, _ := c.rawPost(ctx, "v2/sendinvoice", body)
		tag := "blocked"
		if st >= 200 && st < 300 { tag = ">>> ACCEPTED <<<" }
		short := resp
		if len(short) > 90 { short = short[:90] }
		t.Logf("%s  %-16s status=%d %s", d, tag, st, short)
	}
}
