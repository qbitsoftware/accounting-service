package accounting_test

import (
	"context"
	"fmt"
	"log"
	"time"

	accounting "github.com/qbitsoftware/accounting-service"
	"github.com/shopspring/decimal"
)

// Example demonstrates the complete workflow for creating an invoice with Merit Aktiva:
// 1. Create a customer (if needed)
// 2. Create line items (products/services)
// 3. Generate Estonian reference number (viitenumber)
// 4. Create the invoice
func Example() {
	ctx := context.Background()

	// Create client
	client, err := accounting.NewClient(accounting.Config{
		Provider: "merit",
		APIID:    "your-api-id",
		APIKey:   "your-api-key",
		Region:   "ee", // Estonia
	})
	if err != nil {
		log.Fatal(err)
	}

	// Step 1: Create or find customer
	customer, err := client.Customers.Create(ctx, accounting.CreateCustomerInput{
		Name:        "Acme Corporation OÜ",
		RegNo:       "12345678",
		VATRegNo:    "EE123456789",
		Email:       "billing@acme.ee",
		Phone:       "+372 5555 1234",
		Address:     "Narva mnt 7",
		City:        "Tallinn",
		PostalCode:  "10117",
		CountryCode: "EE",
		Currency:    "EUR",
		Contact:     "John Doe",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Created customer: %s (ID: %s)\n", customer.Name, customer.ID)

	// Step 2: Create items (products/services) if needed
	item, err := client.Items.Create(ctx, accounting.CreateItemInput{
		Code:                "CONSULT-001",
		Description:         "IT Consulting Services",
		Type:                accounting.ItemTypeService,
		UnitOfMeasure:       "hour",
		SalesPrice:          decimal.NewFromFloat(75.00),
		TaxID:               "vat-20", // 20% VAT
		SalesAccountCode:    "3000",
		PurchaseAccountCode: "4000",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Created item: %s (ID: %s)\n", item.Code, item.ID)

	// Step 3: Generate Estonian reference number (viitenumber) using 3-7-1 algorithm
	invoiceNumber := "202501234"
	refNo, err := accounting.GenerateEstonianReference(invoiceNumber)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Generated reference number: %s\n", refNo)

	// Step 4: Create the invoice
	invoice, err := client.Invoices.Create(ctx, accounting.CreateInvoiceInput{
		CustomerID:      customer.ID,
		CustomerName:    customer.Name,
		CustomerRegNo:   customer.RegNo,
		CustomerEmail:   customer.Email,
		CustomerAddress: customer.Address,
		DocDate:         time.Now(),
		DueDate:         time.Now().AddDate(0, 0, 30), // 30 days payment term
		InvoiceNo:       invoiceNumber,
		RefNo:           refNo, // Estonian reference number
		Currency:        "EUR",
		Lines: []accounting.CreateInvoiceLineInput{
			{
				Code:        item.Code,
				Description: item.Description,
				Quantity:    decimal.NewFromInt(40), // 40 hours
				UnitPrice:   item.SalesPrice,        // 75.00 EUR/hour
				TaxID:       item.TaxID,             // 20% VAT
				AccountCode: "3000",
			},
		},
		Comment:       "Monthly IT consulting services",
		FooterComment: "Payment reference: " + refNo,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Created invoice: %s (ID: %s)\n", invoice.Number, invoice.ID)
	totalAmt, _ := invoice.TotalAmount.Float64()
	fmt.Printf("Total amount: %.2f %s\n", totalAmt, invoice.Currency)
}

// ExampleGenerateEstonianReference demonstrates generating Estonian reference numbers
func ExampleGenerateEstonianReference() {
	// Generate reference number for invoice "1234"
	refNo, err := accounting.GenerateEstonianReference("1234")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(refNo)
	// Output: 12344
}

// ExampleValidateEstonianReference demonstrates validating Estonian reference numbers
func ExampleValidateEstonianReference() {
	// Validate a correct reference number
	valid := accounting.ValidateEstonianReference("12344")
	fmt.Println(valid)
	// Output: true
}
