// Command prepay exercises the SimplBooks prepayment (ettemaks) lifecycle
// through the SDK's PrepaymentProvider surface, against a real account:
// park an advance -> list it -> draw it across two invoices -> confirm the
// exhausted advance is reported as such -> tear everything down.
//
//	SIMPLBOOKS_COMPANY_ID=... SIMPLBOOKS_TOKEN=... go run ./simplbooks-probe/prepay
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	accounting "github.com/qbitsoftware/accounting-service"
	"github.com/shopspring/decimal"
)

func main() {
	companyID := os.Getenv("SIMPLBOOKS_COMPANY_ID")
	token := os.Getenv("SIMPLBOOKS_TOKEN")
	if companyID == "" || token == "" {
		log.Fatal("set SIMPLBOOKS_COMPANY_ID and SIMPLBOOKS_TOKEN")
	}
	client, err := accounting.NewClient(accounting.Config{
		Provider: "simplbooks",
		APIID:    companyID,
		APIKey:   token,
		Extra:    map[string]string{"company_id": companyID},
	})
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()
	now := time.Now()

	step := func(name string, err error) bool {
		if err != nil {
			fmt.Printf("  ✗ %-28s %v\n", name, err)
			return false
		}
		fmt.Printf("  ✓ %-28s ok\n", name)
		return true
	}

	// The probe reuses the customer the main probe created.
	const customer = "2"
	const bank = "1" // Kassa
	const taxID = "15"

	mkInvoice := func(name string, amount float64) string {
		inv, err := client.Invoices.Create(ctx, accounting.CreateInvoiceInput{
			CustomerID:   customer,
			CustomerName: "Klubio Testklient OÜ",
			DocDate:      now,
			DueDate:      now.AddDate(0, 0, 14),
			Currency:     "EUR",
			Lines: []accounting.CreateInvoiceLineInput{{
				Description: name,
				Quantity:    decimal.NewFromInt(1),
				UnitPrice:   decimal.NewFromFloat(amount),
				TaxID:       taxID,
				UOMName:     "tk",
			}},
			TotalAmount: decimal.NewFromFloat(amount),
		})
		if err != nil {
			log.Fatalf("create invoice %s: %v", name, err)
		}
		return inv.ID
	}
	showInvoice := func(id, label string) {
		inv, err := client.Invoices.Get(ctx, id)
		if err != nil {
			fmt.Printf("     %s: get failed %v\n", label, err)
			return
		}
		fmt.Printf("     %s: total=%s paid=%s status=%s\n",
			label, inv.TotalAmount, inv.PaidAmount, inv.Status)
	}
	showAdvances := func() {
		pps, err := client.Prepayments.List(ctx, accounting.ListPrepaymentsInput{CustomerCode: customer})
		if err != nil {
			fmt.Printf("     list advances failed: %v\n", err)
			return
		}
		fmt.Printf("     %d advance(s):\n", len(pps))
		for _, pp := range pps {
			fmt.Printf("       no=%s amount=%s remaining=%s comment=%q\n",
				pp.Number, pp.Amount, pp.Remaining, pp.Comment)
		}
	}

	fmt.Println("== park a 50 EUR advance ==")
	pp, err := client.Prepayments.Create(ctx, accounting.CreatePrepaymentInput{
		CustomerCode: customer,
		PrepaymentNo: "KLB-PP-901",
		Amount:       decimal.NewFromFloat(50),
		Currency:     "EUR",
		PaymentDate:  now,
		BankID:       bank,
		Comment:      "Overpayment on invoice INV-TEST",
	})
	if !step("CreatePrepayment", err) {
		return
	}
	fmt.Printf("     advance no=%s amount=%s remaining=%s\n", pp.Number, pp.Amount, pp.Remaining)
	showAdvances()

	fmt.Println("== draw it across two invoices (30 + 20) ==")
	invA := mkInvoice("Treeningtasu A", 30)
	invB := mkInvoice("Treeningtasu B", 20)
	fmt.Printf("     invoice A=%s  invoice B=%s\n", invA, invB)

	step("Apply 30 -> invoice A", client.Prepayments.Apply(ctx, accounting.ApplyPrepaymentInput{
		CustomerCode: customer,
		InvoiceNo:    invA,
		PrepaymentNo: pp.Number,
		Amount:       decimal.NewFromFloat(30),
		Currency:     "EUR",
		PaymentDate:  now,
		BankID:       bank,
	}))
	showInvoice(invA, "A")
	showAdvances()

	step("Apply 20 -> invoice B", client.Prepayments.Apply(ctx, accounting.ApplyPrepaymentInput{
		CustomerCode: customer,
		InvoiceNo:    invB,
		PrepaymentNo: pp.Number,
		Amount:       decimal.NewFromFloat(20),
		Currency:     "EUR",
		PaymentDate:  now,
		BankID:       bank,
	}))
	showInvoice(invB, "B")
	showAdvances()

	fmt.Println("== exhausted advance must NOT silently succeed ==")
	invC := mkInvoice("Treeningtasu C", 15)
	err = client.Prepayments.Apply(ctx, accounting.ApplyPrepaymentInput{
		CustomerCode: customer,
		InvoiceNo:    invC,
		PrepaymentNo: pp.Number,
		Amount:       decimal.NewFromFloat(15),
		Currency:     "EUR",
		PaymentDate:  now,
		BankID:       bank,
	})
	if err == nil {
		fmt.Println("  ✗ over-draw returned success — the no-op guard is NOT working")
	} else {
		fmt.Printf("  ✓ over-draw correctly rejected: %v\n", err)
	}
	showInvoice(invC, "C (must be unpaid)")

	fmt.Println("== teardown ==")
	for _, id := range []string{invA, invB, invC} {
		step("DeleteInvoice "+id, client.Invoices.Delete(ctx, id))
	}
	step("DeletePayment "+pp.Number, client.Payments.Delete(ctx, pp.Number))
}
