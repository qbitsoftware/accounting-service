// Command simplbooks-probe runs a live end-to-end round trip against a real
// SimplBooks account: create customer -> create invoice -> read back -> PDF ->
// pay -> list payments -> then tear everything down again.
//
// Credentials come from SIMPLBOOKS_COMPANY_ID / SIMPLBOOKS_TOKEN.
//
//	go run ./simplbooks-probe
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
	step := func(name string, err error) {
		if err != nil {
			fmt.Printf("  ✗ %-24s %v\n", name, err)
			return
		}
		fmt.Printf("  ✓ %-24s ok\n", name)
	}

	fmt.Println("== reference data ==")
	step("TestConnection", client.TestConnection(ctx))

	taxes, err := client.Taxes.List(ctx)
	step("ListTaxes", err)
	var taxID string
	for _, t := range taxes {
		if t.Name == "Maksuvaba käive" {
			taxID = t.ID
		}
	}
	if taxID == "" && len(taxes) > 0 {
		taxID = taxes[0].ID
	}
	fmt.Printf("     using tax_type_id=%s (of %d)\n", taxID, len(taxes))

	banks, err := client.Taxes.ListBanks(ctx)
	step("ListBanks", err)
	var bankID string
	if len(banks) > 0 {
		bankID = banks[0].ID
		fmt.Printf("     using income_account_id=%s (%s)\n", bankID, banks[0].Name)
	}

	accounts, err := client.Taxes.ListAccounts(ctx)
	step("ListAccounts", err)
	fmt.Printf("     %d financial accounts\n", len(accounts))

	fmt.Println("== customer ==")
	cust, err := client.Customers.FindOrCreate(ctx, "37612a+sbprobe@gmail.com", accounting.CreateCustomerInput{
		Name:        "Klubio Testklient OÜ",
		RegNo:       "99999999",
		Email:       "37612a+sbprobe@gmail.com",
		Address:     "Testtee 1",
		City:        "Tallinn",
		PostalCode:  "10111",
		CountryCode: "EE",
	})
	step("FindOrCreateCustomer", err)
	if err != nil {
		return
	}
	fmt.Printf("     client_id=%s\n", cust.ID)

	custs, err := client.Customers.List(ctx, accounting.ListCustomersInput{})
	step("ListCustomers", err)
	fmt.Printf("     %d customers\n", len(custs))

	fmt.Println("== invoice ==")
	now := time.Now()
	inv, err := client.Invoices.Create(ctx, accounting.CreateInvoiceInput{
		CustomerID:   cust.ID,
		CustomerName: cust.Name,
		DocDate:      now,
		DueDate:      now.AddDate(0, 0, 14),
		RefNo:        "7654323",
		Currency:     "EUR",
		Comment:      "Klubio SimplBooks probe",
		Lines: []accounting.CreateInvoiceLineInput{{
			Description: "Treeningtasu jaanuar",
			Quantity:    decimal.NewFromInt(1),
			UnitPrice:   decimal.NewFromFloat(45.00),
			TaxID:       taxID,
			UOMName:     "tk",
		}},
		TotalAmount: decimal.NewFromFloat(45.00),
	})
	step("CreateInvoice", err)
	if err != nil {
		return
	}
	fmt.Printf("     invoice id=%s no=%s total=%s ref=%q\n",
		inv.ID, inv.Number, inv.TotalAmount, inv.ReferenceNo)

	got, err := client.Invoices.Get(ctx, inv.ID)
	step("GetInvoice", err)
	if err == nil {
		fmt.Printf("     readback total=%s paid=%s lines=%d\n",
			got.TotalAmount, got.PaidAmount, len(got.Lines))
		for _, l := range got.Lines {
			fmt.Printf("       line: %q qty=%s price=%s tax=%s acct=%q\n",
				l.Description, l.Quantity, l.UnitPrice, l.TaxID, l.AccountCode)
		}
	}

	byRef, err := client.Invoices.FindByRef(ctx, "7654323")
	step("FindInvoiceByRef", err)
	if err == nil && byRef != nil {
		fmt.Printf("     found id=%s\n", byRef.ID)
	}

	pdf, err := client.Invoices.GetPDF(ctx, inv.ID, false)
	step("GetInvoicePDF", err)
	if err == nil && pdf != nil {
		fmt.Printf("     pdf %d bytes\n", len(pdf.FileContent))
	}

	fmt.Println("== payment ==")
	err = client.Payments.Create(ctx, accounting.CreatePaymentInput{
		CustomerName: cust.Name,
		CustomerCode: cust.ID,
		InvoiceNo:    inv.ID,
		PaymentNo:    "PROBE-1",
		PaymentDate:  now,
		Amount:       decimal.NewFromFloat(45.00),
		Currency:     "EUR",
		BankID:       bankID,
	})
	step("CreatePayment", err)

	afterPay, err := client.Invoices.Get(ctx, inv.ID)
	step("GetInvoice (after pay)", err)
	if err == nil {
		fmt.Printf("     total=%s paid=%s\n", afterPay.TotalAmount, afterPay.PaidAmount)
	}

	pays, err := client.Payments.List(ctx, accounting.ListPaymentsInput{
		PeriodStart: now.AddDate(0, 0, -7),
		PeriodEnd:   now.AddDate(0, 0, 7),
	})
	step("ListPayments", err)
	fmt.Printf("     %d payments\n", len(pays))
	for _, p := range pays {
		fmt.Printf("       %s %s links=%d id=%s paymode=%s\n",
			p.DocumentDate.Format("2006-01-02"), p.Amount, len(p.InvoiceLinks), p.ID, p.ExternalPayMode)
	}

	fmt.Println("== teardown ==")
	for _, p := range pays {
		if p.ID != "" {
			step("DeletePayment "+p.ID, client.Payments.Delete(ctx, p.ID))
		}
	}
	step("DeleteInvoice", client.Invoices.Delete(ctx, inv.ID))
	fmt.Printf("  ! client_id=%s left in place (no clients/delete in adapter)\n", cust.ID)
}
