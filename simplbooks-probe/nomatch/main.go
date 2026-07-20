package main

// Reproduce the exact failing call: FindCustomerByEmail for an address that
// matches no SimplBooks client (the empty-list envelope case).
import (
	"context"
	"fmt"
	"os"

	accounting "github.com/qbitsoftware/accounting-service"
)

func main() {
	c, err := accounting.NewClient(accounting.Config{
		Provider: "simplbooks",
		APIID:    os.Getenv("SIMPLBOOKS_COMPANY_ID"),
		APIKey:   os.Getenv("SIMPLBOOKS_TOKEN"),
		Extra:    map[string]string{"company_id": os.Getenv("SIMPLBOOKS_COMPANY_ID")},
	})
	if err != nil {
		panic(err)
	}
	ctx := context.Background()

	cust, err := c.Customers.FindOrCreate(ctx, "harrimahar+99@gmail.com", accounting.CreateCustomerInput{
		Name:        "Test test",
		Email:       "harrimahar+99@gmail.com",
		CountryCode: "EE",
	})
	if err != nil {
		fmt.Printf("✗ FindOrCreate: %v\n", err)
		return
	}
	fmt.Printf("✓ FindOrCreate ok — client_id=%s name=%q\n", cust.ID, cust.Name)

	again, err := c.Customers.FindOrCreate(ctx, "harrimahar+99@gmail.com", accounting.CreateCustomerInput{
		Name: "Test test", Email: "harrimahar+99@gmail.com", CountryCode: "EE",
	})
	if err != nil {
		fmt.Printf("✗ second FindOrCreate (should match existing): %v\n", err)
		return
	}
	fmt.Printf("✓ idempotent — resolved to client_id=%s (no duplicate)\n", again.ID)
}
