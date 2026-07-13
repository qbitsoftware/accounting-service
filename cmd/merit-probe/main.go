// Command merit-probe is a READ-ONLY explorer for a club's Merit Aktiva account.
// It dumps everything we can read — chart of accounts, articles (items),
// the customer debt report, unpaid invoices, sampled invoice line items, payments
// and customers — so we can see what a migrated club (e.g. FC Flora) actually has
// in Merit: outstanding debts, and whether invoice lines distinguish fee types
// (training vs facility) via ArticleCode / AccountCode / Description.
//
// It NEVER writes: only List*/Get*/CustomerDebts are called.
//
// Usage:
//
//	MERIT_API_ID=...  MERIT_API_KEY=...  \
//	  [MERIT_API_URL=https://aktiva.merit.ee/api/] \
//	  [MERIT_FROM=20220101] [MERIT_TO=20261231] [MERIT_DEBT_DATE=20260627] \
//	  [MERIT_SAMPLE=8] [MERIT_NAME=] \
//	  go run ./cmd/merit-probe
package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/qbitsoftware/accounting-service/merit"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

const meritDate = "20060102" // Merit's YYYYMMdd date layout

func main() {
	apiID := os.Getenv("MERIT_API_ID")
	apiKey := os.Getenv("MERIT_API_KEY")
	if apiID == "" || apiKey == "" {
		fmt.Println("ERROR: set MERIT_API_ID and MERIT_API_KEY")
		os.Exit(2)
	}

	from := env("MERIT_FROM", time.Now().AddDate(-4, 0, 0).Format(meritDate))
	to := env("MERIT_TO", time.Now().AddDate(0, 4, 0).Format(meritDate))
	debtDate := env("MERIT_DEBT_DATE", time.Now().Format(meritDate))
	sample := 8
	if v := os.Getenv("MERIT_SAMPLE"); v != "" {
		fmt.Sscanf(v, "%d", &sample)
	}
	nameFilter := os.Getenv("MERIT_NAME")

	c := merit.New(merit.Config{
		APIURL: env("MERIT_API_URL", merit.EstoniaURL),
		APIID:  apiID,
		APIKey: apiKey,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Printf("MERIT PROBE (read-only)\n  range %s..%s   debtDate %s   sample %d\n\n", from, to, debtDate, sample)

	// ---- 0) AUTH + chart of accounts ---------------------------------------
	hdr("0) CHART OF ACCOUNTS (auth check)")
	accounts, err := c.ListAccounts(ctx)
	if err != nil {
		fmt.Println("AUTH/READ FAILED:", err)
		fmt.Println("(check MERIT_API_ID / MERIT_API_KEY / MERIT_API_URL — region 'ee' uses the default)")
		os.Exit(1)
	}
	fmt.Printf("auth OK — %d accounts. Revenue/income accounts (codes 3xxx):\n", len(accounts))
	for _, a := range accounts {
		if strings.HasPrefix(a.Code, "3") {
			fmt.Printf("  %-8s %s\n", a.Code, firstNonEmpty(a.Name, a.NameEN))
		}
	}

	// ---- 1) ARTICLES / ITEMS (training vs facility?) -----------------------
	hdr("1) ARTICLES (ITEMS) — do training & facility exist as distinct articles?")
	items, err := c.ListItems(ctx, merit.ListItemsParams{})
	if err != nil {
		fmt.Println("ListItems error:", err)
	} else {
		fmt.Printf("%d articles:\n", len(items))
		for _, it := range items {
			fmt.Printf("  code=%-12s salesAcct=%-8s %-40s [%s]\n",
				it.Code, it.SalesAccountCode, firstNonEmpty(it.Name, it.NameEN), it.TypeName)
		}
	}

	// ---- 2) CUSTOMER DEBT REPORT (the headline) ----------------------------
	hdr("2) CUSTOMER DEBT REPORT (getcustdebtrep) — all outstanding debt")
	debts, err := c.CustomerDebts(ctx, merit.CustomerDebtsParams{DebtDate: debtDate})
	if err != nil {
		fmt.Println("CustomerDebts error:", err)
	} else {
		byType := map[string]struct {
			n      int
			unpaid float64
		}{}
		var totUnpaid float64
		for _, d := range debts {
			u, _ := d.UnPaidAmount.Float64()
			totUnpaid += u
			e := byType[d.DocType]
			e.n++
			e.unpaid += u
			byType[d.DocType] = e
		}
		fmt.Printf("%d debt rows, total unpaid %.2f\n", len(debts), totUnpaid)
		fmt.Println("by DocType (SO=offer, MA=invoice, BA=prepayment, SBx=initial/opening balance):")
		for t, e := range byType {
			fmt.Printf("  %-5s  rows=%-5d  unpaid=%.2f\n", t, e.n, e.unpaid)
		}
		fmt.Println("\nsample rows:")
		printed := 0
		for _, d := range debts {
			if nameFilter != "" && !strings.Contains(strings.ToLower(d.PartnerName), strings.ToLower(nameFilter)) {
				continue
			}
			if printed >= 40 {
				fmt.Printf("  ... (%d more)\n", len(debts)-printed)
				break
			}
			fmt.Printf("  %-28s type=%-4s no=%-10s ref=%-12s due=%-10s total=%8s paid=%8s unpaid=%8s\n",
				trunc(d.PartnerName, 28), d.DocType, d.DocNo, d.RefNo, d.DueDate,
				d.TotalAmount, d.PaidAmount, d.UnPaidAmount)
			printed++
		}
	}

	// ---- 3) UNPAID INVOICES -----------------------------------------------
	hdr("3) UNPAID INVOICES (ListInvoices UnPaid=true)")
	unpaid, err := c.ListInvoices(ctx, merit.ListInvoicesParams{PeriodStart: from, PeriodEnd: to, UnPaid: true})
	if err != nil {
		fmt.Println("ListInvoices(unpaid) error:", err)
	}
	fmt.Printf("%d unpaid invoices in range\n", len(unpaid))
	for i, inv := range unpaid {
		if i >= 15 {
			fmt.Printf("  ... (%d more)\n", len(unpaid)-15)
			break
		}
		fmt.Printf("  %-10s %-26s due=%-10s total=%8s paid=%8s ref=%s\n",
			inv.InvoiceNo, trunc(inv.CustomerName, 26), inv.DueDate, inv.TotalAmount, inv.PaidAmount, inv.ReferenceNo)
	}

	// ---- 4) INVOICE LINE DETAIL (the breakdown evidence) -------------------
	hdr("4) INVOICE LINE ITEMS — sampled detail (ArticleCode / AccountCode / Description)")
	src := unpaid
	if len(src) == 0 {
		// fall back to any invoices in range so we can still see line shapes
		src, _ = c.ListInvoices(ctx, merit.ListInvoicesParams{PeriodStart: from, PeriodEnd: to})
	}
	sort.Slice(src, func(i, j int) bool { return src[i].DocumentDate > src[j].DocumentDate })
	for i, inv := range src {
		if i >= sample {
			break
		}
		det, err := c.GetInvoice(ctx, merit.GetInvoiceParams{ID: inv.SIHId})
		if err != nil {
			fmt.Printf("  GetInvoice(%s) error: %v\n", inv.InvoiceNo, err)
			continue
		}
		fmt.Printf("• %s  %s  total=%s  contract=%q  hcomment=%q\n",
			det.InvoiceNo, det.CustomerName, det.TotalAmount, det.ContractNo, trunc(det.HComment, 40))
		for _, ln := range det.Lines {
			fmt.Printf("    article=%-12s acct=%-8s incl=%8s  %s\n",
				ln.ArticleCode, ln.AccountCode, ln.AmountInclVat, trunc(ln.Description, 50))
		}
	}

	// ---- 5) PAYMENTS -------------------------------------------------------
	hdr("5) PAYMENTS (ListPayments)")
	pays, err := c.ListPayments(ctx, merit.ListPaymentsParams{PeriodStart: from, PeriodEnd: to})
	if err != nil {
		fmt.Println("ListPayments error:", err)
	} else {
		fmt.Printf("%d payments in range\n", len(pays))
		for i, p := range pays {
			if i >= 12 {
				fmt.Printf("  ... (%d more)\n", len(pays)-12)
				break
			}
			fmt.Printf("  %-10s %-26s bank=%-14s amount=%s\n",
				p.DocumentDate, trunc(p.CounterPartName, 26), trunc(p.BankName, 14), p.Amount)
		}
	}

	// ---- 6) CUSTOMERS ------------------------------------------------------
	hdr("6) CUSTOMERS (ListCustomers)")
	custs, err := c.ListCustomers(ctx, merit.ListCustomersParams{Name: nameFilter})
	if err != nil {
		fmt.Println("ListCustomers error:", err)
	} else {
		fmt.Printf("%d customers%s\n", len(custs), iff(nameFilter != "", " matching name filter", ""))
		for i, cu := range custs {
			if i >= 10 {
				fmt.Printf("  ... (%d more)\n", len(custs)-10)
				break
			}
			fmt.Printf("  id=%-12s %-26s reg=%-10s email=%s\n",
				cu.CustomerID, trunc(cu.Name, 26), cu.RegNo, cu.Email)
		}
	}

	fmt.Println("\nDONE (read-only).")
}

func hdr(s string) { fmt.Printf("\n========== %s ==========\n", s) }

func trunc(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func iff(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
