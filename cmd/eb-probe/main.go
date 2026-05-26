// Command eb-probe reproduces the credit-invoice (kreeditarve) creation against
// a real Excellent Books instance to find the exact row format EB accepts.
// Creates UNCONFIRMED drafts (no OKFlag) marked "API PROBE - DELETE" so they're
// easy to remove. Reads credentials from env.
//
//	EB_BASE_URL=https://books124.excellent.ee:3778 \
//	EB_COMPANY=1 EB_USER=... EB_PASS=... \
//	EB_CUST=K7224 EB_ART=ÕPPEMAKS EB_ACC=3702 EB_CREDINV=803 \
//	go run ./cmd/eb-probe
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/qbitsoftware/accounting-service/excellentbooks"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	c := excellentbooks.New(excellentbooks.Config{
		BaseURL:     env("EB_BASE_URL", "https://books124.excellent.ee:3778"),
		CompanyCode: env("EB_COMPANY", "1"),
		Username:    os.Getenv("EB_USER"),
		Password:    os.Getenv("EB_PASS"),
	})

	cust := env("EB_CUST", "K7224")
	art := env("EB_ART", "ÕPPEMAKS")
	acc := env("EB_ACC", "3702")
	credInv := env("EB_CREDINV", "803")

	// 0) Auth + list VAT codes (also gives us a valid VATCode to send).
	codes, _, err := c.ListVATCodes(ctx, excellentbooks.ListParams{Limit: 5000})
	if err != nil {
		fmt.Println("AUTH/LIST FAILED:", err)
		fmt.Println("(check EB_USER / EB_PASS / EB_COMPANY / EB_BASE_URL)")
		os.Exit(1)
	}
	fmt.Printf("AUTH OK — %d VAT codes\n", len(codes))
	vat := os.Getenv("EB_VAT")
	for _, vc := range codes {
		fmt.Printf("  VAT code=%q\n", vc.Code)
		if vat == "" {
			vat = vc.Code // fall back to first; override with EB_VAT
		}
	}
	fmt.Println("using VATCode:", vat, "(override with EB_VAT)")

	base := func() map[string]string {
		return map[string]string{
			"set_field.InvDate":  time.Now().Format("2006-01-02"),
			"set_field.CustCode": cust,
			"set_field.InvType":  "3",
			"set_field.CredMark": "1",
			"set_field.CredInv":  credInv,
			// NO OKFlag -> draft.
			"set_row_field.0.ArtCode":  art,
			"set_row_field.0.SalesAcc": acc,
			"set_row_field.0.Spec":     "API PROBE - DELETE",
		}
	}
	withVat := func(f map[string]string) map[string]string {
		if vat != "" {
			f["set_row_field.0.VATCode"] = vat
		}
		return f
	}

	type variant struct {
		name   string
		mutate func(map[string]string)
	}
	variants := []variant{
		{"A stp=3, Quant=-1 (reproduces prod)", func(f map[string]string) {
			f["set_row_field.0.stp"] = "3"
			f["set_row_field.0.Quant"] = "-1"
			f["set_row_field.0.Price"] = "110"
		}},
		{"B stp=1, Quant=-1 (the stp fix)", func(f map[string]string) {
			f["set_row_field.0.stp"] = "1"
			f["set_row_field.0.Quant"] = "-1"
			f["set_row_field.0.Price"] = "110"
		}},
		{"C stp=1, Quant=1 (positive)", func(f map[string]string) {
			f["set_row_field.0.stp"] = "1"
			f["set_row_field.0.Quant"] = "1"
			f["set_row_field.0.Price"] = "110"
		}},
		{"D no stp, Quant=-1", func(f map[string]string) {
			f["set_row_field.0.Quant"] = "-1"
			f["set_row_field.0.Price"] = "110"
		}},
		{"E stp=3 + OrdRow=1, Quant=-1 (docs' credit-row path)", func(f map[string]string) {
			f["set_row_field.0.stp"] = "3"
			f["set_row_field.0.OrdRow"] = "1"
			f["set_row_field.0.Quant"] = "-1"
			f["set_row_field.0.Price"] = "110"
		}},
	}

	for _, v := range variants {
		f := withVat(base())
		v.mutate(f)
		inv, err := c.CreateInvoice(ctx, f)
		if err != nil {
			fmt.Printf("[%s] -> ERROR: %v\n", v.name, err)
		} else {
			fmt.Printf("[%s] -> OK, created SerNr=%s (DELETE this draft in EB)\n", v.name, inv.SerNr)
		}
	}
}
