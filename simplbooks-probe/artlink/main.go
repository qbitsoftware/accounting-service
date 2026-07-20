package main

// Verify code→article_id resolution end to end, reading article_id back from
// the raw API (the SDK's InvoiceLine doesn't surface it).
import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	accounting "github.com/qbitsoftware/accounting-service"
	"github.com/qbitsoftware/accounting-service/simplbooks"
	"github.com/shopspring/decimal"
)

func main() {
	companyID, token := os.Getenv("SIMPLBOOKS_COMPANY_ID"), os.Getenv("SIMPLBOOKS_TOKEN")
	c, err := accounting.NewClient(accounting.Config{
		Provider: "simplbooks", APIID: companyID, APIKey: token,
		Extra: map[string]string{"company_id": companyID},
	})
	if err != nil {
		log.Fatal(err)
	}
	raw := simplbooks.New(simplbooks.Config{CompanyID: companyID, Token: token})
	ctx := context.Background()
	now := time.Now()

	try := func(label, code string, wantLinked bool) {
		inv, err := c.Invoices.Create(ctx, accounting.CreateInvoiceInput{
			CustomerID: "5", CustomerName: "Test test",
			DocDate: now, DueDate: now.AddDate(0, 0, 14), Currency: "EUR",
			Lines: []accounting.CreateInvoiceLineInput{{
				Code: code, Description: "Liikmemaks — billing",
				Quantity: decimal.NewFromInt(1), UnitPrice: decimal.NewFromFloat(20),
				TaxID: "18", UOMName: "tk",
			}},
			TotalAmount: decimal.NewFromFloat(20),
		})
		if err != nil {
			fmt.Printf("  ✗ %-38s create failed: %v\n", label, err)
			return
		}
		full, err := raw.GetInvoice(ctx, inv.ID)
		if err != nil {
			fmt.Printf("  ✗ %-38s readback failed: %v\n", label, err)
			return
		}
		for _, row := range full.Rows {
			linked := row.ArticleID != 0
			mark := "✓"
			if linked != wantLinked {
				mark = "✗"
			}
			fmt.Printf("  %s %-38s article_id=%-3d code=%-12q account=%d\n",
				mark, label, row.ArticleID, row.Code, row.IncomeAccountID)
		}
		_ = c.Invoices.Delete(ctx, inv.ID)
	}

	fmt.Println("== article code resolution (article_id read from raw API) ==")
	try("'KLB-TEST-1' exists", "KLB-TEST-1", true)
	try("'klb-test-1' case-insensitive", "klb-test-1", true)
	try("'Liikmemaks' no such article", "Liikmemaks", false)
	try("'1001' no such article id", "1001", false)
	try("'2' real article id", "2", true)
}
