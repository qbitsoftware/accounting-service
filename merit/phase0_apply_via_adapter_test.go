package merit

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// TestPhase0SendSettlementClient exercises the typed client method rather than a
// hand-rolled JSON body, so the struct tags, the zero-array handling and the
// yyyyMMdd date format are all proven on the wire — a settlement whose CustLines
// serialise to null is rejected, and that is exactly the kind of fault a raw
// probe passing a hand-written string can never catch.
func TestPhase0SendSettlementClient(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	stamp := time.Now().UTC().Format("150405")
	docDate, err := time.Parse("20060102", "20260603")
	if err != nil {
		t.Fatal(err)
	}
	ppNo := "KLUBIO-CPP-" + stamp

	// Advance of 60 for the customer.
	ppBody := fmt.Sprintf(`{"Description":"typed settlement probe","DocumentNumber":%q,"CurrencyCode":"EUR","DocumentDate":"2026-06-03","Amount":60.00}`, ppNo)
	st, resp, _ := c.rawPost(ctx, fmt.Sprintf("v2/Banks/%s/PrePayments/ForCustomer/%s", phase0SwedBankID, phase0CustomerID), ppBody)
	if st < 200 || st >= 300 {
		t.Fatalf("create prepayment failed: %d %s", st, resp)
	}
	time.Sleep(1500 * time.Millisecond)

	// Unpaid invoice of 60 (net) for the same customer.
	row := fmt.Sprintf(`{"Item":{"Code":%q,"Description":"typed settlement probe"},"Quantity":1,"Price":60.00,"TaxId":%q}`, phase0ItemCode, phase0Tax24)
	invNo := "KLUBIO-CINV-" + stamp
	invBody := fmt.Sprintf(`{"Customer":{"Id":%q},"AccountingDoc":1,"DocDate":"20260603","DueDate":"20260603","InvoiceNo":%q,"CurrencyCode":"EUR","InvoiceRow":[%s],"TaxAmount":[],"TotalAmount":60.00}`,
		phase0CustomerID, invNo, row)
	st2, resp2, _ := c.rawPost(ctx, "v2/sendinvoice", invBody)
	if st2 < 200 || st2 >= 300 {
		t.Fatalf("create invoice failed: %d %s", st2, resp2)
	}
	var im map[string]any
	_ = json.Unmarshal([]byte(resp2), &im)
	invID, _ := im["InvoiceId"].(string)
	time.Sleep(1500 * time.Millisecond)

	// Apply the advance through the typed client.
	amount := decimal.RequireFromString("60.00")
	got, err := c.SendSettlement(ctx, SettlementRequest{
		DocDate:      docDate.Format("20060102"),
		CurrencyCode: "EUR",
		CustLines: []SettlementLine{
			{CustVendID: phase0CustomerID, DocNo: invNo, Amount: amount},
			{CustVendID: phase0CustomerID, DocNo: ppNo, Amount: amount.Neg()},
		},
	})
	if err != nil {
		t.Fatalf("SendSettlement: %v", err)
	}
	t.Logf("SETTLEMENT ok DocumentNo=%s DocumentId=%s", got.DocumentNo, got.DocumentID)
	time.Sleep(2 * time.Second)

	// The invoice must now be fully paid, funded by the advance.
	_, gi, _ := c.rawPost(ctx, "v2/getinvoice", fmt.Sprintf(`{"Id":%q}`, invID))
	var d map[string]any
	_ = json.Unmarshal([]byte(gi), &d)
	hdr, _ := d["Header"].(map[string]any)
	if hdr == nil {
		t.Fatalf("could not read the invoice back: %s", gi)
	}
	t.Logf(">>> INVOICE PaidAmount=%v Paid=%v", hdr["PaidAmount"], hdr["Paid"])
	if paid, _ := hdr["Paid"].(bool); !paid {
		t.Errorf("invoice not paid after settlement; PaidAmount=%v", hdr["PaidAmount"])
	}

	// And the advance must no longer be an open credit on the debt report — that
	// report is what klubio reads advances from, so this is the assertion that
	// matters for the sync, not the settlement's own acknowledgement.
	debts, dErr := c.CustomerDebts(ctx, CustomerDebtsParams{CustID: phase0CustomerID})
	if dErr != nil {
		t.Fatalf("CustomerDebts: %v", dErr)
	}
	for _, it := range debts {
		if it.DocNo == ppNo && it.UnPaidAmount.IsNegative() {
			t.Errorf("advance %s still open on the debt report: UnPaid=%s", ppNo, it.UnPaidAmount)
		}
	}
}
