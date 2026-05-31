// Command eb-offset-experiment reproduces klubio's "credit a PAID invoice" flow
// against Excellent Books and probes the invoice's open state after each step,
// to find why the original invoice is left OPEN in the customer ledger.
//
// Flow: create invoice -> pay in full -> un-allocate (free payment to a
// prepayment, re-open invoice) -> create linked credit note. After each step we
// probe whether the invoice still has open amount by attempting a tiny receipt
// against it (EB error 20060 "amount > invoice amount" == the invoice is closed).
//
//	EB_BASE_URL=https://test.excellent.ee:3490 EB_COMPANY=1 EB_USER=API EB_PASS=SportSoft26 \
//	EB_CUST=107 EB_ART=005 EB_ACC=3100 EB_VAT=1 EB_PAYMODE=P EB_AMOUNT=5 EB_CUPNR=<unique> \
//	go run ./cmd/eb-offset-experiment
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/qbitsoftware/accounting-service/excellentbooks"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

var (
	gc       *excellentbooks.Client
	gCust    string
	gPayMode string
	gToday   string
)

// probeOpen reports the invoice's open amount WITHOUT mutating it: it attempts a
// huge receipt (always rejected) and parses EB's "arve summa <open>" out of the
// 20060 error. A rejection writes nothing, so this is read-only.
func probeOpen(ctx context.Context, invNo string) string {
	_, err := gc.CreateReceipt(ctx, map[string]string{
		"set_field.TransDate":       gToday,
		"set_field.OKFlag":          "1",
		"set_field.PayMode":         gPayMode,
		"set_row_field.0.stp":       "1",
		"set_row_field.0.CustCode":  gCust,
		"set_row_field.0.InvoiceNr": invNo,
		"set_row_field.0.RecVal":    "999999",
		"set_row_field.0.PayDate":   gToday,
	})
	if err == nil {
		return "open >= 999999 (unexpectedly accepted!)"
	}
	const marker = "arve summa "
	msg := err.Error()
	if i := strings.Index(msg, marker); i >= 0 {
		rest := strings.TrimSpace(msg[i+len(marker):])
		// open amount is the number up to the next '.' followed by space, or '.'
		end := strings.IndexAny(rest, " .")
		// allow decimals like 4.99 — take up to first space, then trim trailing dot
		if sp := strings.Index(rest, " "); sp >= 0 {
			end = sp
		}
		open := strings.TrimRight(rest[:end], ".")
		return "open = " + open
	}
	if strings.Contains(msg, "20060") {
		return "open ~0 (closed)"
	}
	return "probe error: " + msg
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	gc = excellentbooks.New(excellentbooks.Config{
		BaseURL:     env("EB_BASE_URL", "https://test.excellent.ee:3490"),
		CompanyCode: env("EB_COMPANY", "1"),
		Username:    os.Getenv("EB_USER"),
		Password:    os.Getenv("EB_PASS"),
	})
	gCust = env("EB_CUST", "107")
	gPayMode = env("EB_PAYMODE", "P")
	art := env("EB_ART", "005")
	acc := env("EB_ACC", "3100")
	vat := env("EB_VAT", "1")
	amount := env("EB_AMOUNT", "5")
	cupnr := env("EB_CUPNR", "910001")
	gToday = time.Now().Format("2006-01-02")

	// Probe-only mode: report an existing invoice's open amount + every receipt
	// row touching it, then exit.
	if pi := os.Getenv("EB_PROBE_INVNO"); pi != "" {
		if inv, err := gc.GetInvoice(ctx, pi); err == nil && inv.CustCode != "" {
			gCust = inv.CustCode // probe receipt needs the invoice's own customer
		}
		fmt.Printf("invoice %s (customer %s): %s\n", pi, gCust, probeOpen(ctx, pi))
		receipts, _, err := gc.ListReceipts(ctx, excellentbooks.ListParams{Limit: 300, Sort: "TransDate"})
		if err != nil {
			fmt.Println("list receipts error:", err)
			return
		}
		var sum float64
		for _, r := range receipts {
			for _, row := range r.Rows {
				if row.InvoiceNr == pi {
					var v float64
					fmt.Sscanf(strings.TrimSpace(row.RecVal), "%f", &v)
					sum += v
					fmt.Printf("  receipt %s row: RecVal=%s CUPNr=%q comment=%q\n", r.SerNr, row.RecVal, row.CUPNr, row.Comment)
				}
			}
		}
		fmt.Printf("  --> net receipts allocated to %s = %.2f\n", pi, sum)
		return
	}

	// Draw mode: simulate the accountant applying an existing ettemaks to a fresh
	// invoice IN EB (two-row receipt: invoice +amount / CUPNr -amount). Reduces
	// the CUPNr's remaining so a klubio pull must follow it down. EB_DRAW_CUPNR +
	// EB_CUST + EB_AMOUNT + EB_ART/ACC/VAT/PAYMODE.
	if drawCUPNr := os.Getenv("EB_DRAW_CUPNR"); drawCUPNr != "" {
		art := env("EB_ART", "005")
		acc := env("EB_ACC", "3100")
		vat := env("EB_VAT", "1")
		amount := env("EB_AMOUNT", "25")
		inv, err := gc.CreateInvoice(ctx, map[string]string{
			"set_field.InvDate": gToday, "set_field.CustCode": gCust, "set_field.OKFlag": "1",
			"set_row_field.0.ArtCode": art, "set_row_field.0.SalesAcc": acc,
			"set_row_field.0.Quant": "1", "set_row_field.0.Price": amount, "set_row_field.0.VATCode": vat,
			"set_row_field.0.Spec": "API EB-SIDE DRAW INV - DELETE",
		})
		if err != nil {
			fmt.Println("draw: create invoice FAILED:", err)
			os.Exit(1)
		}
		rcpt, err := gc.CreateReceipt(ctx, map[string]string{
			"set_field.TransDate": gToday, "set_field.OKFlag": "1", "set_field.PayMode": gPayMode,
			"set_row_field.0.stp": "1", "set_row_field.0.CustCode": gCust,
			"set_row_field.0.InvoiceNr": inv.SerNr, "set_row_field.0.RecVal": amount, "set_row_field.0.PayDate": gToday,
			"set_row_field.1.stp": "1", "set_row_field.1.CustCode": gCust,
			"set_row_field.1.CUPNr": drawCUPNr, "set_row_field.1.RecVal": "-" + amount, "set_row_field.1.PayDate": gToday,
		})
		if err != nil {
			fmt.Println("draw: apply receipt FAILED:", err)
			os.Exit(1)
		}
		fmt.Printf("EB-side draw OK: drew %s from CUPNr %s onto fresh invoice %s (receipt %s)\n", amount, drawCUPNr, inv.SerNr, rcpt.SerNr)
		return
	}

	// 1) invoice
	inv, err := gc.CreateInvoice(ctx, map[string]string{
		"set_field.InvDate": gToday, "set_field.CustCode": gCust, "set_field.OKFlag": "1",
		"set_row_field.0.ArtCode": art, "set_row_field.0.SalesAcc": acc,
		"set_row_field.0.Quant": "1", "set_row_field.0.Price": amount, "set_row_field.0.VATCode": vat,
		"set_row_field.0.Spec": "API OFFSET EXP INV - DELETE",
	})
	if err != nil {
		fmt.Println("create invoice FAILED:", err)
		os.Exit(1)
	}
	fmt.Printf("1) invoice %s (%s). open now: %s\n", inv.SerNr, amount, probeOpen(ctx, inv.SerNr))

	// 2) pay in full
	if _, err := gc.CreateReceipt(ctx, map[string]string{
		"set_field.TransDate": gToday, "set_field.OKFlag": "1", "set_field.PayMode": gPayMode,
		"set_row_field.0.stp": "1", "set_row_field.0.CustCode": gCust,
		"set_row_field.0.InvoiceNr": inv.SerNr, "set_row_field.0.RecVal": amount, "set_row_field.0.PayDate": gToday,
	}); err != nil {
		fmt.Println("pay FAILED:", err)
		os.Exit(1)
	}
	fmt.Printf("2) paid in full. open now: %s\n", probeOpen(ctx, inv.SerNr))

	// 3) un-allocate: -amount off invoice, +amount as prepayment
	if _, err := gc.CreateReceipt(ctx, map[string]string{
		"set_field.TransDate": gToday, "set_field.OKFlag": "1", "set_field.PayMode": gPayMode,
		"set_row_field.0.stp": "1", "set_row_field.0.CustCode": gCust,
		"set_row_field.0.InvoiceNr": inv.SerNr, "set_row_field.0.RecVal": "-" + amount, "set_row_field.0.PayDate": gToday,
		"set_row_field.0.Comment": "API OFFSET EXP unalloc - DELETE",
		"set_row_field.1.stp": "1", "set_row_field.1.CustCode": gCust,
		"set_row_field.1.CUPNr": cupnr, "set_row_field.1.RecVal": amount, "set_row_field.1.PayDate": gToday,
	}); err != nil {
		fmt.Println("un-alloc FAILED:", err)
		os.Exit(1)
	}
	fmt.Printf("3) un-allocated (prepayment %s created). open now: %s\n", cupnr, probeOpen(ctx, inv.SerNr))

	// 4) credit note linked to invoice
	cn, err := gc.CreateInvoice(ctx, map[string]string{
		"set_field.InvDate": gToday, "set_field.CustCode": gCust,
		"set_field.InvType": "3", "set_field.CredMark": "1", "set_field.CredInv": inv.SerNr, "set_field.OKFlag": "1",
		"set_row_field.0.stp": "3", "set_row_field.0.OrdRow": inv.SerNr,
		"set_row_field.1.stp": "1", "set_row_field.1.ArtCode": art, "set_row_field.1.SalesAcc": acc,
		"set_row_field.1.Quant": "1", "set_row_field.1.Price": amount, "set_row_field.1.VATCode": vat,
		"set_row_field.1.Spec": "API OFFSET EXP CN - DELETE",
	})
	if err != nil {
		fmt.Println("credit note FAILED:", err)
		os.Exit(1)
	}
	fmt.Printf("4) credit note %s created. invoice open now: %s\n", cn.SerNr, probeOpen(ctx, inv.SerNr))

	fmt.Println("\nInterpretation:")
	fmt.Println(" - If after step 4 the invoice is OPEN, the un-alloc+credit-note leaves a phantom open invoice (the bug).")
	fmt.Println(" - If CLOSED, the credit note offsets the re-opened invoice and the real-case report needs re-reading.")
	fmt.Println("Clean up 'API OFFSET EXP - DELETE' docs in EB.")
}
