// Command sent verifies live that a SimplBooks invoice can be created already
// flagged as sent — the state klubio needs, since every invoice it pushes has
// already been mailed to the member.
//
// It answers three questions the unit tests cannot:
//   - does invoices/create actually honour the `sent` date, or drop it?
//   - does the adapter's read-back repair kick in when it is dropped?
//   - does POST invoices/sent/{id} work as the standalone repair path?
//
// Credentials come from SIMPLBOOKS_COMPANY_ID / SIMPLBOOKS_TOKEN.
// Every document it creates is deleted again at the end.
//
//	go run ./simplbooks-probe/sent
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
	// Raw client for the read-back: the provider-neutral Invoice type carries no
	// sent flag, and reading the raw field is the whole point of this probe.
	raw := simplbooks.New(simplbooks.Config{CompanyID: companyID, Token: token})

	ctx := context.Background()
	fails := 0
	check := func(name string, ok bool, detail string) {
		mark := "✓"
		if !ok {
			mark, fails = "✗", fails+1
		}
		fmt.Printf("  %s %-38s %s\n", mark, name, detail)
	}

	taxes, err := client.Taxes.List(ctx)
	if err != nil {
		log.Fatalf("ListTaxes: %v", err)
	}
	taxID := ""
	if len(taxes) > 0 {
		taxID = taxes[0].ID
	}

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
	fmt.Printf("client_id=%s\n\n", cust.ID)

	now := time.Now()
	today := now.Format("2006-01-02")
	// A date clearly in the past: if it survives the round trip, the create
	// honoured our value rather than defaulting to "today".
	wantSent := now.AddDate(0, 0, -5)
	wantSentStr := wantSent.Format("2006-01-02")

	line := []accounting.CreateInvoiceLineInput{{
		Description: "Treeningtasu (sent probe)",
		Quantity:    decimal.NewFromInt(1),
		UnitPrice:   decimal.NewFromFloat(10.00),
		TaxID:       taxID,
		UOMName:     "tk",
	}}
	base := accounting.CreateInvoiceInput{
		CustomerID:   cust.ID,
		CustomerName: cust.Name,
		DocDate:      now,
		DueDate:      now.AddDate(0, 0, 14),
		Currency:     "EUR",
		Lines:        line,
		TotalAmount:  decimal.NewFromFloat(10.00),
	}

	var created []string
	defer func() {
		fmt.Println("\n== teardown ==")
		for _, id := range created {
			if err := client.Invoices.Delete(ctx, id); err != nil {
				fmt.Printf("  ! DeleteInvoice %s: %v\n", id, err)
			} else {
				fmt.Printf("  ✓ deleted invoice %s\n", id)
			}
		}
		fmt.Printf("  ! client_id=%s left in place (no clients/delete in the API)\n", cust.ID)
		if fails > 0 {
			fmt.Printf("\nFAILED: %d check(s)\n", fails)
			os.Exit(1)
		}
		fmt.Println("\nALL CHECKS PASSED")
	}()

	// ── 1. create with SentDate ───────────────────────────────────────────────
	fmt.Println("== 1. create with SentDate ==")
	withSent := base
	withSent.SentDate = wantSent
	withSent.Comment = "sent probe: with SentDate"
	inv, err := client.Invoices.Create(ctx, withSent)
	if err != nil {
		log.Fatalf("CreateInvoice (with SentDate): %v", err)
	}
	created = append(created, inv.ID)

	got, err := raw.GetInvoice(ctx, inv.ID)
	if err != nil {
		log.Fatalf("raw GetInvoice: %v", err)
	}
	fmt.Printf("     invoice id=%s no=%s sent=%q\n", inv.ID, inv.Number, got.Sent)
	switch got.Sent {
	case wantSentStr:
		check("create honours `sent`", true, "exact date preserved: "+got.Sent)
	case today:
		// The adapter's read-back repair fired: the flag is right, the date is
		// SimplBooks' own (invoices/sent takes no date argument).
		check("create honours `sent`", true, "repaired via invoices/sent (create dropped the date, flag now "+got.Sent+")")
	default:
		check("create honours `sent`", false, fmt.Sprintf("sent=%q, wanted %q or %q", got.Sent, wantSentStr, today))
	}

	// ── 2. create without SentDate stays unsent ───────────────────────────────
	fmt.Println("\n== 2. create without SentDate ==")
	noSent := base
	noSent.Comment = "sent probe: no SentDate"
	inv2, err := client.Invoices.Create(ctx, noSent)
	if err != nil {
		log.Fatalf("CreateInvoice (no SentDate): %v", err)
	}
	created = append(created, inv2.ID)

	got2, err := raw.GetInvoice(ctx, inv2.ID)
	if err != nil {
		log.Fatalf("raw GetInvoice: %v", err)
	}
	fmt.Printf("     invoice id=%s sent=%q\n", inv2.ID, got2.Sent)
	unsent := got2.Sent == "" || got2.Sent == "0000-00-00"
	check("omitting SentDate leaves it unsent", unsent, fmt.Sprintf("sent=%q", got2.Sent))

	// ── 3. MarkSent repairs it ────────────────────────────────────────────────
	fmt.Println("\n== 3. MarkSent on the unsent invoice ==")
	check("provider advertises MarkSent", client.Invoices.SupportsMarkSent(), "SupportsMarkSent()")
	if err := client.Invoices.MarkSent(ctx, inv2.ID); err != nil {
		check("MarkSent", false, err.Error())
	} else {
		got3, err := raw.GetInvoice(ctx, inv2.ID)
		if err != nil {
			log.Fatalf("raw GetInvoice: %v", err)
		}
		fmt.Printf("     sent=%q\n", got3.Sent)
		nowSent := got3.Sent != "" && got3.Sent != "0000-00-00"
		check("MarkSent flips the flag", nowSent, fmt.Sprintf("sent=%q", got3.Sent))
	}
}
