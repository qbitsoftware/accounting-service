// Command eb-sdk-cn-test exercises the EXACT high-level SDK path klubio uses to
// create a credit note (client.Invoices.CreateCreditNote) and dumps the
// resulting EB document's link row + InvType — to see whether OrdRow reaches EB.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	accounting "github.com/qbitsoftware/accounting-service"
	"github.com/qbitsoftware/accounting-service/excellentbooks"
	"github.com/shopspring/decimal"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cfg := accounting.Config{
		Provider: "excellentbooks",
		APIID:    os.Getenv("EB_USER"),
		APIKey:   os.Getenv("EB_PASS"),
		Extra: map[string]string{
			"base_url":     env("EB_BASE_URL", "https://test.excellent.ee:3490"),
			"company_code": env("EB_COMPANY", "1"),
		},
	}
	client, err := accounting.NewClient(cfg)
	if err != nil {
		fmt.Println("NewClient:", err)
		os.Exit(1)
	}
	raw := excellentbooks.New(excellentbooks.Config{
		BaseURL: cfg.Extra["base_url"], CompanyCode: cfg.Extra["company_code"],
		Username: cfg.APIID, Password: cfg.APIKey,
	})

	cust := env("EB_CUST", "107")
	art := env("EB_ART", "005")
	acc := env("EB_ACC", "3100")
	vat := env("EB_VAT", "1")
	price := decimal.RequireFromString(env("EB_AMOUNT", "7"))

	// 1) Original invoice via the SDK.
	inv, err := client.Invoices.Create(ctx, accounting.CreateInvoiceInput{
		CustomerID: cust, DocDate: time.Now(), DueDate: time.Now(), Currency: "EUR",
		AutoConfirm: true,
		Lines: []accounting.CreateInvoiceLineInput{{
			Code: art, Quantity: decimal.NewFromInt(1), UnitPrice: price,
			TaxID: vat, AccountCode: acc, Description: "SDK CN TEST INV - DELETE",
		}},
	})
	if err != nil {
		fmt.Println("create invoice:", err)
		os.Exit(1)
	}
	fmt.Println("invoice SerNr =", inv.ID)

	// 1b) Replicate klubio's PAID flow: pay in full, then un-allocate to a
	// prepayment (this is what precedes the credit note in merit_sync).
	if os.Getenv("EB_FULLFLOW") == "1" {
		if _, err := raw.CreateReceipt(ctx, map[string]string{
			"set_field.TransDate": time.Now().Format("2006-01-02"), "set_field.OKFlag": "1", "set_field.PayMode": env("EB_PAYMODE", "P"),
			"set_row_field.0.stp": "1", "set_row_field.0.CustCode": cust,
			"set_row_field.0.InvoiceNr": inv.ID, "set_row_field.0.RecVal": price.String(), "set_row_field.0.PayDate": time.Now().Format("2006-01-02"),
		}); err != nil {
			fmt.Println("pay:", err)
			os.Exit(1)
		}
		if _, err := client.Prepayments.Unallocate(ctx, accounting.UnallocateToPrepaymentInput{
			CustomerCode: cust, InvoiceNo: inv.ID, PrepaymentNo: env("EB_CUPNR", "930001"),
			Amount: price, Currency: "EUR", PaymentDate: time.Now(), BankID: env("EB_PAYMODE", "P"),
			Comment: "SDK CN TEST unalloc - DELETE",
		}); err != nil {
			fmt.Println("unalloc:", err)
			os.Exit(1)
		}
		fmt.Println("1b) paid + un-allocated (replicating klubio's paid flow)")
	}

	// 2) Credit note via the SDK — the exact path merit_sync uses.
	cn, err := client.Invoices.CreateCreditNote(ctx, accounting.CreateCreditNoteInput{
		CustomerID: cust, DocDate: time.Now(), DueDate: time.Now(), Currency: "EUR",
		OriginalInvoiceNo: inv.ID,
		TotalAmount:       price.Neg(),
		PaymentTermCode:   os.Getenv("EB_PAYTERM"), // reproduce klubio's PayDeal=S
		Lines: []accounting.CreateInvoiceLineInput{{
			Code: art, Quantity: decimal.NewFromInt(-1), UnitPrice: price,
			TaxID: vat, AccountCode: acc, Description: "SDK CN TEST CN - DELETE",
		}},
	})
	if err != nil {
		fmt.Println("create credit note:", err)
		os.Exit(1)
	}
	fmt.Println("credit note SerNr =", cn.ID)

	// 3) Dump the credit note's raw fields.
	rawData, err := raw.GetRaw(ctx, "IVVc", cn.ID)
	if err != nil {
		fmt.Println("GetRaw:", err)
		os.Exit(1)
	}
	fmt.Println("\nraw credit note:")
	fmt.Println(string(rawData))
}
