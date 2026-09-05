package merit

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// TestPhase0SettlePrepayment answers the question the whole Merit prepayment
// design rests on: can klubio APPLY a customer advance to a specific sales
// invoice through the API, or must the accountant do it by hand in Aktiva?
//
// The adapter has said "no" since it was written (ApplyPrepayment returns
// ErrUnsupportedProvider), and that assumption is expensive: klubio spends the
// credit locally, Merit never learns, and every credit application becomes a
// manual allocation for the club's bookkeeper — 21 of them for Kalevi
// Ujumiskool's first run alone.
//
// But the reference manual documents v2/sendsettlement with Amount signed by
// document type — "SalesInvoice, PurchaseInvoice +, CreditInvoice, Prepayment –"
// — and requires the lines to total zero. That is exactly an offset. The
// existing phase0 probe only ever settled a CREDIT NOTE against an invoice; a
// prepayment was never tried.
//
// So: create an advance, create an unpaid invoice for the same customer, and
// settle the two against each other. Then read both back.
func TestPhase0SettlePrepayment(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	stamp := time.Now().UTC().Format("150405")
	const docDate = "20260603"  // inside the account's open fiscal year
	const ppDate = "2026-06-03" // PrePayments wants yyyy-MM-dd, unlike the rest
	ppNo := "KLUBIO-SPP-" + stamp

	// 1) an advance of 100 for the customer.
	ppBody := fmt.Sprintf(`{"Description":"settle-prepayment probe","DocumentNumber":%q,"CurrencyCode":"EUR","DocumentDate":%q,"Amount":100.00}`, ppNo, ppDate)
	st, resp, _ := c.rawPost(ctx, fmt.Sprintf("v2/Banks/%s/PrePayments/ForCustomer/%s", phase0SwedBankID, phase0CustomerID), ppBody)
	t.Logf("CREATE PREPAYMENT %s -> %d %s", ppNo, st, resp)
	if st < 200 || st >= 300 {
		t.Fatalf("could not create the advance; nothing to settle")
	}
	time.Sleep(1500 * time.Millisecond)

	// 2) an unpaid invoice of 100 (net) for the same customer.
	row := fmt.Sprintf(`{"Item":{"Code":%q,"Description":"settle-prepayment probe"},"Quantity":1,"Price":100.00,"TaxId":%q}`, phase0ItemCode, phase0Tax24)
	invNo := "KLUBIO-SINV-" + stamp
	invBody := fmt.Sprintf(`{"Customer":{"Id":%q},"AccountingDoc":1,"DocDate":%q,"DueDate":%q,"InvoiceNo":%q,"CurrencyCode":"EUR","InvoiceRow":[%s],"TaxAmount":[],"TotalAmount":100.00}`,
		phase0CustomerID, docDate, docDate, invNo, row)
	st2, resp2, _ := c.rawPost(ctx, "v2/sendinvoice", invBody)
	t.Logf("CREATE INVOICE %s -> %d %s", invNo, st2, resp2)
	var im map[string]any
	_ = json.Unmarshal([]byte(resp2), &im)
	invID, _ := im["InvoiceId"].(string)
	createdInvNo, _ := im["InvoiceNo"].(string)
	if createdInvNo == "" {
		createdInvNo = invNo
	}
	if invID == "" {
		t.Fatalf("could not create the invoice; nothing to settle")
	}
	time.Sleep(1500 * time.Millisecond)

	// 3) settle them. Try the documented signing first (invoice +, prepayment −);
	// fall back to both-positive, which is what the credit-note probe used and
	// suggests Merit may derive direction from the document type itself.
	attempts := []struct {
		label     string
		invAmt    string
		prepayAmt string
	}{
		{"invoice +100 / prepayment -100 (as documented)", "100.00", "-100.00"},
		{"both +100 (direction from doc type)", "100.00", "100.00"},
	}
	settled := false
	for _, a := range attempts {
		body := fmt.Sprintf(`{"DocDate":%q,"CurrencyCode":"EUR","CustLines":[{"CustVendName":%q,"CustVendId":%q,"DocNo":%q,"Amount":%s},{"CustVendName":%q,"CustVendId":%q,"DocNo":%q,"Amount":%s}],"VendLines":[]}`,
			docDate,
			phase0CustomerName, phase0CustomerID, createdInvNo, a.invAmt,
			phase0CustomerName, phase0CustomerID, ppNo, a.prepayAmt)
		sst, sresp, _ := c.rawPost(ctx, "v2/sendsettlement", body)
		t.Logf("SETTLEMENT [%s] -> %d %s", a.label, sst, sresp)
		if sst >= 200 && sst < 300 {
			settled = true
			break
		}
	}
	time.Sleep(2 * time.Second)

	// 4) the answer: is the invoice now paid, and is the advance consumed?
	_, gi, _ := c.rawPost(ctx, "v2/getinvoice", fmt.Sprintf(`{"Id":%q}`, invID))
	var d map[string]any
	_ = json.Unmarshal([]byte(gi), &d)
	if hdr, ok := d["Header"].(map[string]any); ok {
		t.Logf(">>> INVOICE PaidAmount=%v Paid=%v  (settlement accepted=%v)", hdr["PaidAmount"], hdr["Paid"], settled)
	}

	// The customer-debt report is where klubio reads advances from, so check the
	// advance there rather than trusting the settlement's response alone: a BA
	// row whose UnPaidAmount has gone to zero is what "consumed" looks like to us.
	debts, err := c.CustomerDebts(ctx, CustomerDebtsParams{CustID: phase0CustomerID})
	if err != nil {
		t.Logf("CustomerDebts failed: %v", err)
		return
	}
	for _, it := range debts {
		if it.DocNo == ppNo || it.DocNo == createdInvNo {
			t.Logf(">>> DEBT ROW DocNo=%s DocType=%s Total=%s UnPaid=%s",
				it.DocNo, it.DocType, it.TotalAmount, it.UnPaidAmount)
		}
	}
}
