package main

// Mimic UpsertArticleInProvider against SimplBooks: List by code → Create if
// missing → confirm a line then resolves to it. Then rename and re-resolve.
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
	companyID, token := os.Getenv("SIMPLBOOKS_COMPANY_ID"), os.Getenv("SIMPLBOOKS_TOKEN")
	c, err := accounting.NewClient(accounting.Config{
		Provider: "simplbooks", APIID: companyID, APIKey: token,
		Extra: map[string]string{"company_id": companyID},
	})
	if err != nil {
		log.Fatal(err)
	}
	raw := simplbooks.New(simplbooks.Config{CompanyID: companyID, Token: token})
	ctx := context.Background()

	const code = "KLB-UPSERT-PROBE"
	const renamed = "KLB-UPSERT-PROBE2"

	// Step 1: lookup by code (what UpsertArticleInProvider does first)
	items, err := c.Items.List(ctx, accounting.ListItemsInput{Code: code})
	if err != nil {
		log.Fatalf("Items.List: %v", err)
	}
	found := ""
	for _, it := range items {
		if it.Code == code {
			found = it.ID
		}
	}
	fmt.Printf("  lookup %q -> %s\n", code, map[bool]string{true: "exists " + found, false: "not found"}[found != ""])

	// Step 2: create only when missing — the same branch UpsertArticleInProvider takes
	created := &accounting.Item{ID: found, Code: code}
	if found == "" {
		created, err = c.Items.Create(ctx, accounting.CreateItemInput{
			Code: code, Description: "Klubio upsert probe",
			Type: accounting.ItemTypeService,
			// klubio passes these; SimplBooks ignores both (read-only on its schema)
			SalesAccountCode: "102", TaxID: "18",
		})
		if err != nil {
			log.Fatalf("Items.Create: %v", err)
		}
		fmt.Printf("  ✓ created article id=%s code=%q\n", created.ID, created.Code)
	} else {
		fmt.Printf("  · reusing existing article id=%s\n", created.ID)
	}

	// Step 3: a line carrying the readable code must now resolve to it
	fmt.Println("  resolving a line with that code…")
	if err := checkResolve(ctx, c, raw, code, created.ID); err != nil {
		fmt.Printf("  ✗ %v\n", err)
	}

	// Step 4: rename (UpsertArticleInProvider handles code changes)
	newCode := renamed
	desc := "Klubio upsert probe renamed"
	if err := c.Items.Update(ctx, accounting.UpdateItemInput{
		ID: created.ID, Code: &newCode, Description: &desc,
	}); err != nil {
		fmt.Printf("  ✗ Items.Update: %v\n", err)
	} else {
		fmt.Printf("  ✓ renamed to %q\n", renamed)
		if err := checkResolve(ctx, c, raw, renamed, created.ID); err != nil {
			fmt.Printf("  ✗ after rename: %v\n", err)
		}
	}

	fmt.Printf("  ! leaving article %s in place — delete it in SimplBooks if unwanted\n", created.ID)
}

func nowDate() time.Time { return time.Now() }
func dec(i int64) decimal.Decimal { return decimal.NewFromInt(i) }

func checkResolve(ctx context.Context, c *accounting.Client, raw *simplbooks.Client, code, wantID string) error {
	inv, err := c.Invoices.Create(ctx, accounting.CreateInvoiceInput{
		CustomerID: "5", CustomerName: "Test test",
		DocDate: nowDate(), DueDate: nowDate(), Currency: "EUR",
		Lines: []accounting.CreateInvoiceLineInput{{
			Code: code, Description: "probe line",
			Quantity: dec(1), UnitPrice: dec(1), TaxID: "18", UOMName: "tk",
		}},
		TotalAmount: dec(1),
	})
	if err != nil {
		return fmt.Errorf("create invoice: %w", err)
	}
	defer func() { _ = c.Invoices.Delete(ctx, inv.ID) }()
	full, err := raw.GetInvoice(ctx, inv.ID)
	if err != nil {
		return fmt.Errorf("readback: %w", err)
	}
	for _, r := range full.Rows {
		if fmt.Sprint(r.ArticleID) != wantID {
			return fmt.Errorf("line linked to article_id=%d, want %s", r.ArticleID, wantID)
		}
		fmt.Printf("  ✓ line linked: article_id=%d code=%q\n", r.ArticleID, r.Code)
	}
	return nil
}
