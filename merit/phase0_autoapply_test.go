package merit

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// Does a customer prepayment auto-apply to a newly created invoice?
func TestPhase0AutoApply(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	stamp := time.Now().UTC().Format("150405")

	// 1) fresh prepayment 100 for the customer (PrePayments wants yyyy-MM-dd)
	ppBody := fmt.Sprintf(`{"Description":"autoapply test","DocumentNumber":"KLUBIO-AAPP-%s","CurrencyCode":"EUR","DocumentDate":"2026-06-03","Amount":100.00}`, stamp)
	st, resp, _ := c.rawPost(ctx, fmt.Sprintf("v2/Banks/%s/PrePayments/ForCustomer/%s", phase0SwedBankID, phase0CustomerID), ppBody)
	t.Logf("CREATE PREPAYMENT -> %d %s", st, resp)

	time.Sleep(1500 * time.Millisecond)

	// 2) new invoice 100 for the same customer (sendinvoice wants yyyyMMdd)
	row := fmt.Sprintf(`{"Item":{"Code":%q,"Description":"autoapply"},"Quantity":1,"Price":100.00,"TaxId":%q}`, phase0ItemCode, phase0Tax24)
	invBody := fmt.Sprintf(`{"Customer":{"Id":%q},"AccountingDoc":1,"DocDate":"20260603","DueDate":"20260603","InvoiceNo":"KLUBIO-AAINV-%s","CurrencyCode":"EUR","InvoiceRow":[%s],"TaxAmount":[],"TotalAmount":100.00}`,
		phase0CustomerID, stamp, row)
	st2, resp2, _ := c.rawPost(ctx, "v2/sendinvoice", invBody)
	t.Logf("CREATE INVOICE -> %d %s", st2, resp2)
	var m map[string]any
	_ = json.Unmarshal([]byte(resp2), &m)
	invID, _ := m["InvoiceId"].(string)

	time.Sleep(2 * time.Second)

	// 3) is the invoice auto-paid from the prepayment?
	_, gi, _ := c.rawPost(ctx, "v2/getinvoice", fmt.Sprintf(`{"Id":%q}`, invID))
	var d map[string]any
	_ = json.Unmarshal([]byte(gi), &d)
	if hdr, ok := d["Header"].(map[string]any); ok {
		t.Logf(">>> NEW INVOICE PaidAmount=%v Paid=%v  (if >0, Merit auto-applied the prepayment)", hdr["PaidAmount"], hdr["Paid"])
	}
}
