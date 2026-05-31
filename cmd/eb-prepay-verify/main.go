// Command eb-prepay-verify exercises the high-level PrepaymentService against a
// live EB test instance: create a prepayment, then list prepayments and confirm
// the new CUPNr appears with the right remaining balance. Confirms the adapter
// wiring + ListPrepayments aggregation, not just the raw wire format.
//
//	EB_BASE_URL=https://test.excellent.ee:3490 EB_COMPANY=1 EB_USER=API EB_PASS=... \
//	EB_CUST=107 EB_PAYMODE=P EB_CUPNR=VERIFY001 EB_AMOUNT=6 \
//	go run ./cmd/eb-prepay-verify
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	accounting "github.com/qbitsoftware/accounting-service"
	"github.com/shopspring/decimal"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := accounting.NewClient(accounting.Config{
		Provider: "excellentbooks",
		APIID:    os.Getenv("EB_USER"),
		APIKey:   os.Getenv("EB_PASS"),
		Extra: map[string]string{
			"base_url":     env("EB_BASE_URL", "https://test.excellent.ee:3490"),
			"company_code": env("EB_COMPANY", "1"),
		},
	})
	if err != nil {
		fmt.Println("NewClient failed:", err)
		os.Exit(1)
	}

	fmt.Println("Prepayments supported:", client.Prepayments.Supported())

	cust := env("EB_CUST", "107")
	cupnr := env("EB_CUPNR", "VERIFY001")
	amount, _ := decimal.NewFromString(env("EB_AMOUNT", "6"))
	payMode := env("EB_PAYMODE", "P")

	pp, err := client.Prepayments.Create(ctx, accounting.CreatePrepaymentInput{
		CustomerCode: cust,
		PrepaymentNo: cupnr,
		Amount:       amount,
		Currency:     "EUR",
		PaymentDate:  time.Now(),
		BankID:       payMode,
		Comment:      "API PREPAY VERIFY - DELETE",
	})
	if err != nil {
		fmt.Println("Create prepayment FAILED:", err)
		os.Exit(1)
	}
	fmt.Printf("Created prepayment CUPNr=%s amount=%s\n", pp.Number, pp.Amount)

	list, err := client.Prepayments.List(ctx, accounting.ListPrepaymentsInput{
		CustomerCode: cust,
		Since:        time.Now().AddDate(0, 0, -2),
		Until:        time.Now().AddDate(0, 0, 1),
	})
	if err != nil {
		fmt.Println("List prepayments FAILED:", err)
		os.Exit(1)
	}
	fmt.Printf("ListPrepayments returned %d for customer %s:\n", len(list), cust)
	found := false
	for _, p := range list {
		mark := ""
		if p.Number == cupnr {
			mark = "  <-- the one we just created"
			found = true
		}
		fmt.Printf("  CUPNr=%-12s amount=%-8s remaining=%-8s cur=%s%s\n", p.Number, p.Amount, p.Remaining, p.Currency, mark)
	}
	if !found {
		fmt.Println("WARNING: our new CUPNr did not appear in ListPrepayments (check window/customer filter)")
	}
	fmt.Println("\nDONE. Delete the 'API PREPAY VERIFY - DELETE' receipt in EB.")
}
