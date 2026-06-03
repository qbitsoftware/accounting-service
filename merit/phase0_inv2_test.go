package merit

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestPhase0Inv2(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	row := fmt.Sprintf(`{"Item":{"Code":%q,"Description":"x"},"Quantity":1,"Price":100.00,"TaxId":%q}`, phase0ItemCode, phase0Tax24)
	tax := fmt.Sprintf(`{"TaxId":%q,"Amount":24.00}`, phase0Tax24)

	// try with explicit TransactionDate too, single in-range date
	for _, d := range []string{"2026-06-03", "2026-01-02", "2025-03-15"} {
		body := fmt.Sprintf(`{"Customer":{"Id":%q},"AccountingDoc":1,"DocDate":%q,"DueDate":%q,"TransactionDate":%q,"InvoiceNo":"KLUBIO-INV2-%s","CurrencyCode":"EUR","InvoiceRow":[%s],"TaxAmount":[%s],"TotalAmount":124.00}`,
			phase0CustomerID, d, d, d, time.Now().UTC().Format("0405"), row, tax)
		st, resp, _ := c.rawPost(ctx, "v2/sendinvoice", body)
		t.Logf("INVOICE @%s -> status=%d resp=%s", d, st, resp)
	}

	// sanity: does THIS api company still have my test prepayments? (same company check)
	p, _ := c.ListPayments(ctx, ListPaymentsParams{PeriodStart: "20260501", PeriodEnd: "20260731"})
	t.Logf("prepayments visible to this API key in Jun-2026 window: %d", len(p))
	for _, x := range p {
		t.Logf("  - DocNo=%q Amount=%s Date=%s", x.DocumentNo, x.Amount, x.DocumentDate)
	}
}
