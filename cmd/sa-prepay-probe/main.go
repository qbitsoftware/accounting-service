// Command sa-prepay-probe validates the SmartAccounts prepayment lifecycle
// THROUGH THE accounting.Client FACADE — the exact layer klubio's backend uses.
// It parks an advance, lists it, applies it to an invoice (net-zero), verifies
// the invoice closes and the advance is consumed, and checks that Unallocate is
// unsupported. Everything it creates is deleted afterwards (best-effort).
//
// Env: SA_API_KEY, SA_SECRET_KEY (required); SA_BANK (default "Swedbank");
// SA_PREPAY_ACCOUNT (default "21211"); SA_ARTICLE (default "00010").
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	accounting "github.com/qbitsoftware/accounting-service"
	"github.com/qbitsoftware/accounting-service/smartaccounts"
	"github.com/shopspring/decimal"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func check(label string, cond bool) {
	mark := "✓ PASS"
	if !cond {
		mark = "✗ FAIL"
	}
	fmt.Printf("  %s  %s\n", mark, label)
}

func main() {
	ctx := context.Background()
	apiKey, secret := os.Getenv("SA_API_KEY"), os.Getenv("SA_SECRET_KEY")
	bank := env("SA_BANK", "Swedbank")
	account := env("SA_PREPAY_ACCOUNT", "21211")
	article := env("SA_ARTICLE", "00010")

	client, err := accounting.NewClient(accounting.Config{Provider: "smartaccounts", APIID: apiKey, APIKey: secret})
	if err != nil {
		fmt.Printf("new client: %v\n", err)
		os.Exit(1)
	}
	// Raw client for setup (client/invoice) and cleanup (payment/invoice delete).
	raw := smartaccounts.New(smartaccounts.Config{APIKey: apiKey, SecretKey: secret})

	now := time.Now()
	today := now.Format("02.01.2006")
	stamp := now.Format("150405")

	fmt.Println("=== SmartAccounts prepayment lifecycle (via facade) ===")
	check("Prepayments.Supported()", client.Prepayments.Supported())

	// setup: client + a 10.00 invoice
	cust, err := raw.CreateClient(ctx, smartaccounts.CreateClientRequest{Name: "PP VERIFY " + stamp})
	if err != nil {
		fmt.Printf("create client: %v\n", err)
		os.Exit(1)
	}
	inv, err := raw.CreateInvoice(ctx, smartaccounts.CreateInvoiceRequest{
		ClientID: cust.ClientID, Date: today, DueDate: today, Currency: "EUR",
		Rows: []smartaccounts.InvoiceRowInput{{Code: article, Description: "pp verify line", Price: decimal.NewFromInt(10), Quantity: decimal.NewFromInt(1), VatPc: "0"}},
	})
	if err != nil {
		fmt.Printf("create invoice: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  setup: client=%s invoice=%s (%s)\n", cust.ClientID, inv.InvoiceID, inv.InvoiceNumber)

	// 1) PARK a 10.00 advance — embed the klubio token in the description exactly
	// as the backend's saCreatePrepayment does (so it can be resolved back).
	token := "KLB-PP-" + stamp
	pp, err := client.Prepayments.Create(ctx, accounting.CreatePrepaymentInput{
		CustomerCode: cust.ClientID, PrepaymentNo: token,
		Amount: decimal.NewFromInt(10), Currency: "EUR", PaymentDate: now,
		BankID: bank, Account: account, Comment: token + " Overpayment on invoice " + string(inv.InvoiceNumber),
	})
	if err != nil {
		fmt.Printf("  ✗ CreatePrepayment: %v\n", err)
		cleanup(ctx, raw, cust.ClientID, inv.InvoiceID, today)
		os.Exit(1)
	}
	check("CreatePrepayment returns SA paymentId as Number", pp.Number != "")
	fmt.Printf("      parked id=%s remaining=%s\n", pp.Number, pp.Remaining)

	// 2) LIST — advance shows with remaining 10
	list, err := client.Prepayments.List(ctx, accounting.ListPrepaymentsInput{CustomerCode: cust.ClientID})
	if err != nil {
		fmt.Printf("  ✗ ListPrepayments: %v\n", err)
	}
	// resolve token → SA paymentId exactly as saApplyCreditViaPrepayment does
	resolvedID := ""
	for _, l := range list {
		if strings.HasPrefix(l.Comment, token+" ") || l.Comment == token {
			resolvedID = l.Number
			check("List shows parked advance, remaining=10.00", l.Remaining.Equal(decimal.NewFromInt(10)))
		}
	}
	check("token resolves to SA paymentId (backend apply path)", resolvedID != "" && resolvedID == pp.Number)

	// 3) APPLY to the invoice (net-zero) using the TOKEN-RESOLVED id — should close it
	if err := client.Prepayments.Apply(ctx, accounting.ApplyPrepaymentInput{
		CustomerCode: cust.ClientID, InvoiceNo: string(inv.InvoiceNumber), PrepaymentNo: resolvedID,
		Amount: decimal.NewFromInt(10), Currency: "EUR", PaymentDate: now, BankID: bank,
	}); err != nil {
		fmt.Printf("  ✗ ApplyPrepayment: %v\n", err)
	}
	after, _ := client.Invoices.Get(ctx, inv.InvoiceID)
	check("invoice CLOSED after apply (outstanding=0)", after != nil && after.Status == accounting.InvoiceStatusPaid)
	if after != nil {
		fmt.Printf("      invoice status=%s total=%s paid=%s\n", after.Status, after.TotalAmount, after.PaidAmount)
	}

	// 4) LIST again — advance consumed
	list2, _ := client.Prepayments.List(ctx, accounting.ListPrepaymentsInput{CustomerCode: cust.ClientID})
	stillOpen := false
	for _, l := range list2 {
		if l.Number == pp.Number {
			stillOpen = true
		}
	}
	check("advance consumed (no longer listed)", !stillOpen)

	// 5) Unallocate is unsupported
	_, uerr := client.Prepayments.Unallocate(ctx, accounting.UnallocateToPrepaymentInput{
		CustomerCode: cust.ClientID, InvoiceNo: string(inv.InvoiceNumber), PrepaymentNo: "x", Amount: decimal.NewFromInt(1),
	})
	check("Unallocate returns ErrUnsupportedProvider", errors.Is(uerr, accounting.ErrUnsupportedProvider))

	// cleanup
	fmt.Println("=== cleanup ===")
	cleanup(ctx, raw, cust.ClientID, inv.InvoiceID, today)
	fmt.Println("done.")
}

// cleanup deletes every payment booked today against clientID, then the invoice.
func cleanup(ctx context.Context, raw *smartaccounts.Client, clientID, invID, today string) {
	pays, _, err := raw.ListPayments(ctx, smartaccounts.ListPaymentsParams{DateFrom: today, DateTo: today, FetchRows: true})
	if err == nil {
		for _, p := range pays {
			if p.ClientID != clientID {
				continue
			}
			if derr := raw.DeletePayment(ctx, p.ID); derr != nil {
				fmt.Printf("  ! delete payment %s: %v\n", p.ID, derr)
			} else {
				fmt.Printf("  deleted payment %s\n", p.ID)
			}
		}
	}
	if invID != "" {
		if derr := raw.DeleteInvoice(ctx, invID); derr != nil {
			fmt.Printf("  ! delete invoice %s: %v\n", invID, derr)
		} else {
			fmt.Printf("  deleted invoice %s\n", invID)
		}
	}
	fmt.Printf("  (client %s left in place)\n", clientID)
}
