// Command seed creates ONE invoice, already flagged as sent, and leaves it in
// place so the sent state can be eyeballed in the SimplBooks UI.
//
//	SIMPLBOOKS_COMPANY_ID=... SIMPLBOOKS_TOKEN=... go run ./simplbooks-probe/seed
package main

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
	raw := simplbooks.New(simplbooks.Config{CompanyID: companyID, Token: token})
	ctx := context.Background()

	taxes, err := client.Taxes.List(ctx)
	if err != nil {
		log.Fatalf("ListTaxes: %v", err)
	}
	taxID := ""
	if len(taxes) > 0 {
		taxID = taxes[0].ID
	}

	// Reuse the probe's client card rather than adding another undeletable one.
	cust, err := client.Customers.FindOrCreate(ctx, "37612a+sbsent@gmail.com", accounting.CreateCustomerInput{
		Name:        "Klubio Sent-Probe OÜ",
		RegNo:       "99999998",
		Email:       "37612a+sbsent@gmail.com",
		Address:     "Testtee 2",
		City:        "Tallinn",
		PostalCode:  "10111",
		CountryCode: "EE",
	})
	if err != nil {
		log.Fatalf("FindOrCreateCustomer: %v", err)
	}

	now := time.Now()
	sent := now.AddDate(0, 0, -3) // a past date, to show the real send date survives

	// SimplBooks validates the 7-3-1 check digit and rejects a hand-picked
	// number ("Sisestatud viitenumber ei ole korrektne").
	refNo, err := accounting.GenerateEstonianReference("765433")
	if err != nil {
		log.Fatalf("GenerateEstonianReference: %v", err)
	}

	inv, err := client.Invoices.Create(ctx, accounting.CreateInvoiceInput{
		CustomerID:   cust.ID,
		CustomerName: cust.Name,
		DocDate:      now.AddDate(0, 0, -3),
		DueDate:      now.AddDate(0, 0, 11),
		SentDate:     sent,
		RefNo:        refNo,
		Currency:     "EUR",
		Comment:      "Klubio demo — pushed already marked as sent",
		Lines: []accounting.CreateInvoiceLineInput{{
			Description: "Treeningtasu juuli 2026",
			Quantity:    decimal.NewFromInt(1),
			UnitPrice:   decimal.NewFromFloat(45.00),
			TaxID:       taxID,
			UOMName:     "tk",
		}},
		TotalAmount: decimal.NewFromFloat(45.00),
		AutoConfirm: true,
	})
	if err != nil {
		log.Fatalf("CreateInvoice: %v", err)
	}

	got, err := raw.GetInvoice(ctx, inv.ID)
	if err != nil {
		log.Fatalf("raw GetInvoice: %v", err)
	}
	fmt.Printf("\nseeded and LEFT IN PLACE:\n  id=%s  number=%s  total=%s  ref=%s\n  sent=%q  due=%q\n",
		inv.ID, inv.Number, inv.TotalAmount, inv.ReferenceNo, got.Sent, got.Due)
}
