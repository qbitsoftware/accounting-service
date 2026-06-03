package merit

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

const (
	phase0Tax24    = "1e420e04-3dd7-46a5-b71f-0490779c2638"
	phase0ItemCode = "telefon"
)

func (c *Client) jpost(ctx context.Context, t *testing.T, label, ep, body string) (int, map[string]any) {
	t.Helper()
	st, resp, err := c.rawPost(ctx, ep, body)
	if err != nil {
		t.Fatalf("%s transport: %v", label, err)
	}
	t.Logf("%s -> status=%d\n  resp=%s", label, st, resp)
	var m map[string]any
	_ = json.Unmarshal([]byte(resp), &m)
	return st, m
}

// TestPhase0Invoice exercises the Merit-native credit/apply path:
// create invoice -> pay it -> create credit note -> settle them via sendsettlement.
func TestPhase0Invoice(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Empty TaxAmount -> Merit computes VAT from the row TaxId; TotalAmount is the NET rows sum.
	row := fmt.Sprintf(`{"Item":{"Code":%q,"Description":"Klubio Phase0 teenus"},"Quantity":1,"Price":100.00,"TaxId":%q}`, phase0ItemCode, phase0Tax24)
	stamp := time.Now().UTC().Format("150405")

	// 1) create sales invoice, hunting for a DocDate inside the active fiscal year.
	// NOTE: sendinvoice/sendpayment expect yyyyMMdd (meritDateFormat), NOT yyyy-MM-dd.
	candidates := []string{"20260603", "20260102", "20270115", "20250602"}
	var docDate, invNo, invID string
	for _, d := range candidates {
		due := d // same-day due is fine
		body := fmt.Sprintf(`{"Customer":{"Id":%q},"AccountingDoc":1,"DocDate":%q,"DueDate":%q,"InvoiceNo":"KLUBIO-INV-%s","CurrencyCode":"EUR","InvoiceRow":[%s],"TaxAmount":[],"TotalAmount":100.00}`,
			phase0CustomerID, d, due, stamp, row)
		st, m := c.jpost(ctx, t, "CREATE INVOICE @"+d, "v2/sendinvoice", body)
		if st >= 200 && st < 300 {
			docDate = d
			invNo, _ = m["InvoiceNo"].(string)
			invID, _ = m["InvoiceId"].(string)
			break
		}
	}
	t.Logf(">> docDate=%q sales invoice No=%q Id=%q", docDate, invNo, invID)
	if invNo == "" {
		t.Fatal("could not create an invoice in any candidate fiscal year; stopping")
	}

	// 2) PAY invoice A fully (amount must match the 100 total, not 124) -> tests sendpayment
	payBody := fmt.Sprintf(`{"BankId":%q,"CustomerName":%q,"InvoiceNo":%q,"PaymentDate":%q,"Amount":100.00,"CurrencyCode":"EUR"}`,
		phase0SwedBankID, phase0CustomerName, invNo, docDate)
	c.jpost(ctx, t, "PAY INVOICE A (100)", "v2/sendpayment", payBody)
	time.Sleep(1500 * time.Millisecond)
	_, gotA := c.jpost(ctx, t, "GET INVOICE A (post-pay, expect Paid)", "v2/getinvoice", fmt.Sprintf(`{"Id":%q}`, invID))
	if hdr, ok := gotA["Header"].(map[string]any); ok {
		t.Logf(">>> INVOICE A PaidAmount=%v Paid=%v", hdr["PaidAmount"], hdr["Paid"])
	}

	// 3) create a SECOND invoice C (unpaid) to settle a credit note against
	invCBody := fmt.Sprintf(`{"Customer":{"Id":%q},"AccountingDoc":1,"DocDate":%q,"DueDate":%q,"InvoiceNo":"KLUBIO-INVC-%s","CurrencyCode":"EUR","InvoiceRow":[%s],"TaxAmount":[],"TotalAmount":100.00}`,
		phase0CustomerID, docDate, docDate, stamp, row)
	_, invC := c.jpost(ctx, t, "CREATE INVOICE C (unpaid, 100)", "v2/sendinvoice", invCBody)
	invCNo, _ := invC["InvoiceNo"].(string)

	// 4) create a credit note B (kreeditarve, 100) -> tests credit note creation
	credBody := fmt.Sprintf(`{"Customer":{"Id":%q},"AccountingDoc":5,"DocDate":%q,"DueDate":%q,"InvoiceNo":"KLUBIO-CN-%s","CurrencyCode":"EUR","InvoiceRow":[%s],"TaxAmount":[],"TotalAmount":100.00}`,
		phase0CustomerID, docDate, docDate, stamp, row)
	_, cred := c.jpost(ctx, t, "CREATE CREDIT NOTE B (100)", "v2/sendinvoice", credBody)
	credNo, _ := cred["InvoiceNo"].(string)

	// 5) settle credit note B against invoice C (net zero, 100 each) -> tests sendsettlement apply
	if credNo != "" && invCNo != "" {
		// Both magnitudes positive — API likely derives direction from doc type (invoice vs credit).
		settleBody := fmt.Sprintf(`{"DocDate":%q,"CurrencyCode":"EUR","CustLines":[{"CustVendName":%q,"CustVendRegNo":"","DocNo":%q,"Amount":100.00},{"CustVendName":%q,"CustVendRegNo":"","DocNo":%q,"Amount":100.00}],"VendLines":[]}`,
			docDate, phase0CustomerName, invCNo, phase0CustomerName, credNo)
		c.jpost(ctx, t, "SETTLEMENT B vs C (both +100)", "v2/sendsettlement", settleBody)
		c.jpost(ctx, t, "GET INVOICE C (post-settle)", "v2/getinvoice", fmt.Sprintf(`{"Id":%q}`, invC["InvoiceId"]))
	}

	// 6) inspect both invoices afterward
	c.jpost(ctx, t, "GET INVOICE A (final)", "v2/getinvoice", fmt.Sprintf(`{"Id":%q}`, invID))
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -2)
	pays, _ := c.ListPayments(ctx, ListPaymentsParams{PeriodStart: start.Format("20060102"), PeriodEnd: end.Format("20060102")})
	dump(t, "PAYMENTS (recent)", pays)
}
