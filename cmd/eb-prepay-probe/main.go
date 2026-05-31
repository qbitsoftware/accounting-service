// Command eb-prepay-probe investigates how an Excellent Books instance represents
// and applies customer prepayments (ettemaks), so we can design the klubio <-> EB
// prepayment sync correctly.
//
// It answers three questions:
//  1. Does EB expose an invoice's OPEN/PAID amount anywhere in the API payload?
//     (The typed Invoice struct only has Sum4 = total; the "open amount" in the
//     "Sum4 > invoice open amount" error is computed server-side — is it returned?)
//  2. Does a customer record carry a prepayment/balance field?
//  3. (opt-in writes) Will EB accept an UNLINKED credit note (no CredInv) and an
//     UNALLOCATED receipt (no InvoiceNr) as ways to record a carry-forward credit,
//     and does it then auto-allocate to open invoices?
//
// READ-ONLY by default. Writes happen only with EB_PROBE_WRITE=1, are confirmed
// (OKFlag=1 — required for prepayment behaviour to manifest) and tagged
// "API PREPAY PROBE - DELETE" so they're easy to find and remove in EB.
//
//	EB_BASE_URL=https://books124.excellent.ee:3778 \
//	EB_COMPANY=1 EB_USER=... EB_PASS=... \
//	EB_CUST=K7224 EB_INVNO=803 \
//	[EB_RECEIPT=...] \
//	[EB_PROBE_WRITE=1 EB_ART=ÕPPEMAKS EB_ACC=3702 EB_VAT=... EB_PAYMODE=P1 EB_AMOUNT=5] \
//	go run ./cmd/eb-prepay-probe
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/qbitsoftware/accounting-service/excellentbooks"
)

const (
	registerInvoice  = "IVVc"
	registerReceipt  = "IPVc"
	registerCustomer = "CUVc"
)

// interesting flags fields whose name hints at open/paid/balance/prepayment so
// they stand out in the dump.
var interesting = []string{"open", "paid", "rest", "bal", "saldo", "prepay", "ettemaks", "advance", "credit", "kred", "due", "owed", "sum"}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	c := excellentbooks.New(excellentbooks.Config{
		BaseURL:     env("EB_BASE_URL", "https://books124.excellent.ee:3778"),
		CompanyCode: env("EB_COMPANY", "1"),
		Username:    os.Getenv("EB_USER"),
		Password:    os.Getenv("EB_PASS"),
	})

	cust := env("EB_CUST", "K7224")
	invNo := os.Getenv("EB_INVNO")

	// Auth check.
	if _, _, err := c.ListVATCodes(ctx, excellentbooks.ListParams{Limit: 1}); err != nil {
		fmt.Println("AUTH FAILED:", err)
		fmt.Println("(check EB_USER / EB_PASS / EB_COMPANY / EB_BASE_URL)")
		os.Exit(1)
	}
	fmt.Println("AUTH OK")

	// ---- READ PHASE ----------------------------------------------------------

	if invNo != "" {
		fmt.Printf("\n========== INVOICE %s (looking for open/paid amount) ==========\n", invNo)
		raw, err := c.GetRaw(ctx, registerInvoice, invNo)
		if err != nil {
			fmt.Println("  GetRaw invoice ERROR:", err)
		} else {
			dumpRecord(raw, registerInvoice)
		}
	} else {
		fmt.Println("\n========== RECENT INVOICES (dumping raw fields to find open/paid) ==========")
		if raw, err := c.ListRaw(ctx, registerInvoice, excellentbooks.ListParams{Limit: 5}); err != nil {
			fmt.Println("  ListRaw invoices ERROR:", err)
		} else {
			dumpRecord(raw, registerInvoice)
		}
	}

	fmt.Printf("\n========== CUSTOMER %s (looking for prepayment/balance) ==========\n", cust)
	if raw, err := c.GetRaw(ctx, registerCustomer, cust); err != nil {
		fmt.Println("  GetRaw customer ERROR:", err)
	} else {
		dumpRecord(raw, registerCustomer)
	}

	if rcpt := os.Getenv("EB_RECEIPT"); rcpt != "" {
		fmt.Printf("\n========== RECEIPT %s (row structure / prepayment markers) ==========\n", rcpt)
		if raw, err := c.GetRaw(ctx, registerReceipt, rcpt); err != nil {
			fmt.Println("  GetRaw receipt ERROR:", err)
		} else {
			dumpRecord(raw, registerReceipt)
		}
	}

	// A few recent receipts — look for any unallocated row (no InvoiceNr) that EB
	// already uses to record prepayments.
	fmt.Println("\n========== RECENT RECEIPTS (scanning for unallocated rows) ==========")
	if raw, err := c.ListRaw(ctx, registerReceipt, excellentbooks.ListParams{Limit: 25}); err != nil {
		fmt.Println("  ListRaw receipts ERROR:", err)
	} else {
		scanReceiptsForPrepay(raw)
	}

	// ---- WRITE PHASE (opt-in) ------------------------------------------------

	if os.Getenv("EB_PROBE_WRITE") != "1" {
		fmt.Println("\n(write tests skipped — set EB_PROBE_WRITE=1 with EB_ART/EB_ACC/EB_VAT/EB_PAYMODE to test unlinked credit note + unallocated receipt)")
		return
	}

	art := env("EB_ART", "ÕPPEMAKS")
	acc := env("EB_ACC", "3702")
	vat := os.Getenv("EB_VAT")
	payMode := env("EB_PAYMODE", "P1")
	amount := env("EB_AMOUNT", "5")
	today := time.Now().Format("2006-01-02")

	fmt.Println("\n========== WRITE TEST 1: UNLINKED credit note (no CredInv) ==========")
	cnFields := map[string]string{
		"set_field.InvDate":  today,
		"set_field.CustCode": cust,
		"set_field.InvType":  "3", // kreeditarve
		"set_field.CredMark": "1",
		// NO set_field.CredInv  -> standalone credit, should become customer prepayment
		"set_field.OKFlag":         "1",
		"set_row_field.0.stp":      "1",
		"set_row_field.0.ArtCode":  art,
		"set_row_field.0.SalesAcc": acc,
		"set_row_field.0.Quant":    "1",
		"set_row_field.0.Price":    amount,
		"set_row_field.0.Spec":     "API PREPAY PROBE - DELETE",
	}
	if vat != "" {
		cnFields["set_row_field.0.VATCode"] = vat
	}
	if inv, err := c.CreateInvoice(ctx, cnFields); err != nil {
		fmt.Println("  UNLINKED credit note REJECTED:", err)
	} else {
		fmt.Printf("  UNLINKED credit note ACCEPTED — SerNr=%s (DELETE in EB). Re-dumping customer to spot balance change:\n", inv.SerNr)
		if raw, err := c.GetRaw(ctx, registerCustomer, cust); err == nil {
			dumpRecord(raw, registerCustomer)
		}
	}

	fmt.Println("\n========== WRITE TEST 2: UNALLOCATED receipt (no InvoiceNr) ==========")
	rFields := map[string]string{
		"set_field.TransDate":      today,
		"set_field.OKFlag":         "1",
		"set_field.PayMode":        payMode,
		"set_row_field.0.stp":      "1",
		"set_row_field.0.CustCode": cust,
		"set_row_field.0.RecVal":   amount,
		"set_row_field.0.PayDate":  today,
		"set_row_field.0.Comment":  "API PREPAY PROBE - DELETE",
		// NO set_row_field.0.InvoiceNr -> unallocated; should sit as customer prepayment.
		// EB demands a prepayment number (CUPNr) for an unallocated receipt row.
		"set_row_field.0.CUPNr": env("EB_CUPNR", "900001"),
	}
	if rcpt, err := c.CreateReceipt(ctx, rFields); err != nil {
		fmt.Println("  UNALLOCATED receipt REJECTED:", err)
	} else {
		fmt.Printf("  UNALLOCATED receipt ACCEPTED — SerNr=%s (DELETE in EB)\n", rcpt.SerNr)
	}

	// WRITE TEST 3: apply an EXISTING prepayment (CUPNr) to a fresh invoice.
	// This is the "€20 case" mechanism: does EB let us draw down a prepayment to
	// settle an invoice via a two-row receipt (row0 pays invoice, row1 draws CUPNr)?
	applyCUPNr := os.Getenv("EB_APPLY_CUPNR")
	if applyCUPNr != "" {
		fmt.Println("\n========== WRITE TEST 3: apply prepayment CUPNr to a fresh invoice ==========")
		invFields := map[string]string{
			"set_field.InvDate":        today,
			"set_field.CustCode":       cust,
			"set_field.OKFlag":         "1",
			"set_row_field.0.ArtCode":  art,
			"set_row_field.0.SalesAcc": acc,
			"set_row_field.0.Quant":    "1",
			"set_row_field.0.Price":    amount,
			"set_row_field.0.Spec":     "API PREPAY PROBE INV - DELETE",
		}
		if vat != "" {
			invFields["set_row_field.0.VATCode"] = vat
		}
		inv, err := c.CreateInvoice(ctx, invFields)
		if err != nil {
			fmt.Println("  could not create test invoice:", err)
		} else {
			fmt.Printf("  test invoice created SerNr=%s — applying prepayment %s via two-row receipt\n", inv.SerNr, applyCUPNr)
			applyFields := map[string]string{
				"set_field.TransDate": today,
				"set_field.OKFlag":    "1",
				"set_field.PayMode":   payMode,
				// row0: pay the invoice
				"set_row_field.0.stp":       "1",
				"set_row_field.0.CustCode":  cust,
				"set_row_field.0.InvoiceNr": inv.SerNr,
				"set_row_field.0.RecVal":    amount,
				"set_row_field.0.PayDate":   today,
				// row1: draw the same amount from the existing prepayment (negative)
				"set_row_field.1.stp":      "1",
				"set_row_field.1.CustCode": cust,
				"set_row_field.1.CUPNr":    applyCUPNr,
				"set_row_field.1.RecVal":   "-" + amount,
				"set_row_field.1.PayDate":  today,
			}
			if rcpt, err := c.CreateReceipt(ctx, applyFields); err != nil {
				fmt.Println("  PREPAYMENT APPLICATION REJECTED:", err)
			} else {
				fmt.Printf("  PREPAYMENT APPLICATION ACCEPTED — receipt SerNr=%s (net cash 0; invoice settled from prepayment)\n", rcpt.SerNr)
			}
		}
	}

	// WRITE TEST 4: full "credit a PAID invoice" flow (Piece 1).
	//   1. create invoice, 2. pay it in full (open -> 0),
	//   3. un-allocate via a receipt (row0 InvoiceNr -amount, row1 CUPNr +amount)
	//      -> invoice re-opens, freed money becomes a prepayment,
	//   4. issue the linked credit note that fails on a paid invoice — should now
	//      succeed because the invoice has open amount again.
	if os.Getenv("EB_FULLFLOW") == "1" {
		fmt.Println("\n========== WRITE TEST 4: credit a PAID invoice (un-allocate + linked credit note) ==========")
		invFields := map[string]string{
			"set_field.InvDate":        today,
			"set_field.CustCode":       cust,
			"set_field.OKFlag":         "1",
			"set_row_field.0.ArtCode":  art,
			"set_row_field.0.SalesAcc": acc,
			"set_row_field.0.Quant":    "1",
			"set_row_field.0.Price":    amount,
			"set_row_field.0.Spec":     "API PREPAY PROBE INV - DELETE",
		}
		if vat != "" {
			invFields["set_row_field.0.VATCode"] = vat
		}
		inv, err := c.CreateInvoice(ctx, invFields)
		if err != nil {
			fmt.Println("  step1 create invoice FAILED:", err)
		} else {
			fmt.Printf("  step1 invoice created SerNr=%s\n", inv.SerNr)

			// step2: pay in full
			if _, err := c.CreateReceipt(ctx, map[string]string{
				"set_field.TransDate":       today,
				"set_field.OKFlag":          "1",
				"set_field.PayMode":         payMode,
				"set_row_field.0.stp":       "1",
				"set_row_field.0.CustCode":  cust,
				"set_row_field.0.InvoiceNr": inv.SerNr,
				"set_row_field.0.RecVal":    amount,
				"set_row_field.0.PayDate":   today,
			}); err != nil {
				fmt.Println("  step2 pay invoice FAILED:", err)
			} else {
				fmt.Println("  step2 invoice paid in full (open -> 0)")
			}

			// step3: un-allocate — negative RecVal against the invoice + prepayment row
			freeCUPNr := env("EB_FREE_CUPNR", "900050")
			if _, err := c.CreateReceipt(ctx, map[string]string{
				"set_field.TransDate":       today,
				"set_field.OKFlag":          "1",
				"set_field.PayMode":         payMode,
				"set_row_field.0.stp":       "1",
				"set_row_field.0.CustCode":  cust,
				"set_row_field.0.InvoiceNr": inv.SerNr,
				"set_row_field.0.RecVal":    "-" + amount,
				"set_row_field.0.PayDate":   today,
				"set_row_field.0.Comment":   "API PREPAY PROBE unalloc - DELETE",
				"set_row_field.1.stp":       "1",
				"set_row_field.1.CustCode":  cust,
				"set_row_field.1.CUPNr":     freeCUPNr,
				"set_row_field.1.RecVal":    amount,
				"set_row_field.1.PayDate":   today,
			}); err != nil {
				fmt.Println("  step3 UN-ALLOCATION REJECTED:", err)
			} else {
				fmt.Printf("  step3 UN-ALLOCATION ACCEPTED — invoice re-opened, freed money in prepayment %s\n", freeCUPNr)
			}

			// step4: the linked credit note that fails on a paid invoice
			cnFields := map[string]string{
				"set_field.InvDate":        today,
				"set_field.CustCode":       cust,
				"set_field.InvType":        "3",
				"set_field.CredMark":       "1",
				"set_field.CredInv":        inv.SerNr,
				"set_field.OKFlag":         "1",
				"set_row_field.0.stp":      "3",
				"set_row_field.0.OrdRow":   inv.SerNr,
				"set_row_field.1.stp":      "1",
				"set_row_field.1.ArtCode":  art,
				"set_row_field.1.SalesAcc": acc,
				"set_row_field.1.Quant":    "1",
				"set_row_field.1.Price":    amount,
				"set_row_field.1.Spec":     "API PREPAY PROBE CN - DELETE",
			}
			if vat != "" {
				cnFields["set_row_field.1.VATCode"] = vat
			}
			if cn, err := c.CreateInvoice(ctx, cnFields); err != nil {
				fmt.Println("  step4 LINKED CREDIT NOTE REJECTED:", err)
			} else {
				fmt.Printf("  step4 LINKED CREDIT NOTE ACCEPTED — SerNr=%s. FULL FLOW WORKS.\n", cn.SerNr)
			}
		}
	}

	fmt.Println("\nDONE. Remember to delete the 'API PREPAY PROBE - DELETE' documents in EB.")
}

// dumpRecord pretty-prints the first record of the register envelope, sorting
// keys and flagging interesting ones.
func dumpRecord(raw json.RawMessage, register string) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		fmt.Println("  (could not parse envelope):", string(raw))
		return
	}
	arr, ok := envelope[register]
	if !ok {
		fmt.Println("  (register key not found, raw payload):")
		fmt.Println(indent(string(raw)))
		return
	}
	var recs []map[string]any
	if err := json.Unmarshal(arr, &recs); err != nil || len(recs) == 0 {
		fmt.Println("  (no records)")
		return
	}
	rec := recs[0]
	keys := make([]string, 0, len(rec))
	for k := range rec {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k == "rows" {
			continue // rows printed separately
		}
		v := rec[k]
		mark := ""
		lk := strings.ToLower(k)
		for _, hint := range interesting {
			if strings.Contains(lk, hint) {
				mark = "   <-- LOOK"
				break
			}
		}
		fmt.Printf("  %-18s = %v%s\n", k, v, mark)
	}
	if rows, ok := rec["rows"]; ok {
		fmt.Printf("  rows: %v\n", rows)
	}
}

// scanReceiptsForPrepay lists recent receipts and prints any row without an
// InvoiceNr (a candidate prepayment representation).
func scanReceiptsForPrepay(raw json.RawMessage) {
	var envelope struct {
		IPVc []excellentbooks.Receipt `json:"IPVc"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		fmt.Println("  (parse error):", err)
		return
	}
	found := 0
	for _, r := range envelope.IPVc {
		for _, row := range r.Rows {
			if strings.TrimSpace(row.InvoiceNr) == "" {
				found++
				fmt.Printf("  UNALLOCATED row: receipt %s cust=%s val=%s comment=%q\n", r.SerNr, row.CustCode, row.RecVal, row.Comment)
			}
		}
	}
	if found == 0 {
		fmt.Println("  (no unallocated receipt rows in the last 25 — EB may not use this for prepayments, or there simply are none)")
	}
}

func indent(s string) string {
	return "    " + strings.ReplaceAll(s, "\n", "\n    ")
}
