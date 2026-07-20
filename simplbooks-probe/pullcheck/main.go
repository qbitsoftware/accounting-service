package main

// Verify ListInvoices now reports the PARTIAL payment on INV000117 (15 of 30),
// which the raw list projection hides.
import (
	"context"
	"fmt"
	"os"
	"time"

	accounting "github.com/qbitsoftware/accounting-service"
)

func main() {
	companyID, token := os.Getenv("SIMPLBOOKS_COMPANY_ID"), os.Getenv("SIMPLBOOKS_TOKEN")
	c, _ := accounting.NewClient(accounting.Config{
		Provider: "simplbooks", APIID: companyID, APIKey: token,
		Extra: map[string]string{"company_id": companyID},
	})
	ctx := context.Background()
	now := time.Now()
	invs, err := c.Invoices.List(ctx, accounting.ListInvoicesInput{
		PeriodStart: now.AddDate(0, 0, -90),
		PeriodEnd:   now.AddDate(0, 0, 90),
	})
	if err != nil {
		fmt.Println("✗", err)
		return
	}
	for _, i := range invs {
		fmt.Printf("  %-12s total=%-7s paid=%-7s status=%-8s\n", i.Number, i.TotalAmount, i.PaidAmount, i.Status)
	}
}
