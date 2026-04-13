// directo-probe creates a single barebones unconfirmed invoice via XML
// Direct so you can verify end-to-end write access works.
//
// The invoice is left UNCONFIRMED so it's easy to delete from Directo's
// UI afterwards (won't post to the ledger).
//
// Usage:
//
//	DIRECTO_COMPANY_CODE=xxx \
//	DIRECTO_XML_TOKEN=yyy \
//	DIRECTO_TEST_CUSTOMER_CODE=zzz \
//	    go run ./cmd/directo-probe
//
// DIRECTO_TEST_CUSTOMER_CODE must reference a customer that already
// exists in your Directo (since the customer write component is not
// licensed on your token).
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/qbitsoftware/accounting-service/directo"
)

func main() {
	company := os.Getenv("DIRECTO_COMPANY_CODE")
	token := os.Getenv("DIRECTO_XML_TOKEN")
	customerCode := os.Getenv("DIRECTO_TEST_CUSTOMER_CODE")

	if company == "" || token == "" {
		fmt.Fprintln(os.Stderr, "DIRECTO_COMPANY_CODE and DIRECTO_XML_TOKEN must be set")
		os.Exit(2)
	}
	if customerCode == "" {
		fmt.Fprintln(os.Stderr, "DIRECTO_TEST_CUSTOMER_CODE must be set — use a customer code that already exists in your Directo")
		os.Exit(2)
	}

	client, err := directo.New(directo.Config{
		Company: company,
		Token:   token,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "client init:", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Generate a unique invoice number. Directo's `number` field is xsd:int
	// (length 9), so we take the unix timestamp mod 1e9 — collision-resistant
	// enough for a probe and well within the 9-digit cap.
	invoiceNumber := fmt.Sprintf("%d", time.Now().Unix()%1_000_000_000)

	inv := directo.InvoiceXML{
		Number:       invoiceNumber,
		CustomerCode: customerCode,
		CustomerName: "Probe Customer OÜ",
		Date:         time.Now().Format("2006-01-02"),
		Currency:     "EUR",

		// Inline customer details. Without these Directo rejects the invoice
		// with type="13" desc="Missing customer email or reg.no" whenever the
		// customercode doesn't already have email/reg.no on file in Directo's
		// customer master.
		Email:         "probe@example.com",
		CustomerRegNo: "10000000",
		CustomerType:  "0", // 0=company

		// Confirm intentionally left empty — keeps the invoice as a draft
		// so it's safe to delete from the Directo UI after the probe.
		Rows: directo.NewInvoiceRows([]directo.InvoiceLineXML{
			{
				Description: "Probe line — safe to delete",
				Quantity:    "1",
				Price:       "0.01",
			},
		}),
	}

	fmt.Printf("Submitting invoice number=%s customer=%s (draft, unconfirmed)\n\n", invoiceNumber, customerCode)

	results, err := client.CreateInvoice(ctx, inv, nil)
	if err != nil {
		fmt.Println("ERROR from SDK:", err)
		os.Exit(1)
	}

	fmt.Println("\nDirecto results:")
	for _, r := range results.Results {
		fmt.Printf("  type=%q desc=%q\n", r.Type, r.Desc)
		if r.Type == "0" {
			fmt.Printf("\n✓ Invoice %s accepted. Look for it in Directo (it will be a draft you can delete).\n", invoiceNumber)
		}
	}
}
