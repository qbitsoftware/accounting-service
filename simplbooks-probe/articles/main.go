// Command articles probes how SimplBooks resolves an invoice line's revenue
// account: from the line's explicit account, from a referenced article, or
// derived from the VAT type. This is what klubio's article/GL routing depends on.
//
//	SIMPLBOOKS_COMPANY_ID=... SIMPLBOOKS_TOKEN=... go run ./simplbooks-probe/articles
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
		Provider: "simplbooks", APIID: companyID, APIKey: token,
		Extra: map[string]string{"company_id": companyID},
	})
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()
	now := time.Now()
	const customer = "2"

	// Reference data: pick a zero-rate and a standard-rate VAT type, and two
	// distinct revenue accounts, so we can tell which input drove the posting.
	accounts, err := client.Taxes.ListAccounts(ctx)
	if err != nil {
		log.Fatalf("ListAccounts: %v", err)
	}
	byCode := map[string]accounting.Account{}
	for _, a := range accounts {
		byCode[a.Code] = a
	}
	svcSales := byCode["4200"] // Teenuste müük Eestis
	vatFree := byCode["4400"]  // Käibemaksuvaba käive
	fmt.Printf("accounts: 4200 id=%s  4400 id=%s\n", svcSales.ID, vatFree.ID)

	fmt.Println("\n== 1. article create: which fields actually stick? ==")
	item, err := client.Items.Create(ctx, accounting.CreateItemInput{
		Code:             "KLB-TEST-1",
		Description:      "Klubio testartikkel",
		UnitOfMeasure:    "tk",
		TaxID:            "15",
		SalesAccountCode: svcSales.ID, // try to give the article a sales account
	})
	if err != nil {
		fmt.Printf("  ✗ CreateItem: %v\n", err)
	} else {
		fmt.Printf("  ✓ created article id=%s code=%q name=%q tax=%q\n",
			item.ID, item.Code, item.Name, item.TaxID)
		fmt.Println("    ^ if tax/account came back empty, SimplBooks ignored them (read-only)")
	}

	mk := func(label string, line accounting.CreateInvoiceLineInput) {
		inv, err := client.Invoices.Create(ctx, accounting.CreateInvoiceInput{
			CustomerID: customer, CustomerName: "Klubio Testklient OÜ",
			DocDate: now, DueDate: now.AddDate(0, 0, 14), Currency: "EUR",
			Lines:       []accounting.CreateInvoiceLineInput{line},
			TotalAmount: line.UnitPrice.Mul(line.Quantity),
		})
		if err != nil {
			fmt.Printf("  ✗ %-38s %v\n", label, err)
			return
		}
		got, err := client.Invoices.Get(ctx, inv.ID)
		if err != nil {
			fmt.Printf("  ✗ %-38s readback: %v\n", label, err)
			return
		}
		for _, l := range got.Lines {
			fmt.Printf("  ✓ %-38s → account=%-5s tax=%-3s (invoice %s)\n",
				label, l.AccountCode, l.TaxID, inv.ID)
		}
		_ = client.Invoices.Delete(ctx, inv.ID)
	}

	fmt.Println("\n== 2. what drives the line's revenue account? ==")
	base := func() accounting.CreateInvoiceLineInput {
		return accounting.CreateInvoiceLineInput{
			Description: "Treeningtasu", Quantity: decimal.NewFromInt(1),
			UnitPrice: decimal.NewFromFloat(10), UOMName: "tk",
		}
	}

	l := base()
	l.TaxID = "15" // Maksuvaba käive
	mk("VAT-free tax only, no account", l)

	l = base()
	l.TaxID = "1" // 20% Teenuste müük, Eesti
	mk("20% services tax only, no account", l)

	l = base()
	l.TaxID = "15"
	l.AccountCode = svcSales.ID // explicit override
	mk("VAT-free tax + explicit account 4200", l)

	if item != nil {
		l = base()
		l.TaxID = "15"
		l.Code = item.ID // reference the article by its numeric id
		mk("article reference, no explicit account", l)

		l = base()
		l.TaxID = "15"
		l.Code = item.ID
		l.AccountCode = svcSales.ID
		mk("article + explicit account 4200", l)
	}

	fmt.Println("\n(articles are left in place — SimplBooks has no articles/delete in the adapter)")
}
