package main

// Replicate exactly what UpsertArticleInProvider does for the article the UI
// just saved: List by code -> no match -> Create with the same inputs klubio passes.
import (
	"context"
	"fmt"
	"os"

	accounting "github.com/qbitsoftware/accounting-service"
)

func main() {
	companyID, token := os.Getenv("SIMPLBOOKS_COMPANY_ID"), os.Getenv("SIMPLBOOKS_TOKEN")
	c, err := accounting.NewClient(accounting.Config{
		Provider: "simplbooks", APIID: companyID, APIKey: token,
		Extra: map[string]string{"company_id": companyID},
	})
	if err != nil {
		panic(err)
	}
	ctx := context.Background()

	const code = "Liikmemaks2"
	items, err := c.Items.List(ctx, accounting.ListItemsInput{Code: code})
	if err != nil {
		fmt.Printf("✗ Items.List: %v\n", err)
		return
	}
	fmt.Printf("  Items.List returned %d article(s)\n", len(items))
	for _, it := range items {
		if it.Code == code {
			fmt.Printf("  already exists: id=%s\n", it.ID)
			return
		}
	}

	// Exactly the input klubio builds: SalesAccountCode from the article's
	// DefaultAccountCode ("1010"), TaxID empty (no per-article tax field).
	created, err := c.Items.Create(ctx, accounting.CreateItemInput{
		Code:             code,
		Description:      "Liikmemaks",
		Type:             accounting.ItemTypeService,
		SalesAccountCode: "1010",
		TaxID:            "",
	})
	if err != nil {
		fmt.Printf("✗ Items.Create: %v\n", err)
		return
	}
	fmt.Printf("  ✓ created id=%s code=%q\n", created.ID, created.Code)
}
