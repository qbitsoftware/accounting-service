# Accounting Service

A Go library providing accounting and bookkeeping functionality with support for multiple accounting service providers (SimplBooks, Merit).

## Installation

```bash
go get github.com/qbitsoftware/accounting-service
```

## Usage

### Creating a Client

```go
import "github.com/qbitsoftware/accounting-service"

// Configure Merit Aktiva client for Estonia
client, err := accounting.NewClient(accounting.Config{
    Provider: "merit",
    APIID:    "your-api-id",
    APIKey:   "your-api-key",
    Region:   "ee", // or "pl" for Poland
})
if err != nil {
    log.Fatal(err)
}

// Test connection
if err := client.TestConnection(ctx); err != nil {
    log.Fatal(err)
}
```

### Working with Invoices

```go
// Generate Estonian reference number (viitenumber)
refNo, err := accounting.GenerateEstonianReference("202501234")
// Returns: "2025012343"

// Create an invoice
invoice, err := client.Invoices.Create(ctx, accounting.CreateInvoiceInput{
    CustomerID:   "customer-123",
    CustomerName: "Acme Corp",
    DocDate:      time.Now(),
    DueDate:      time.Now().AddDate(0, 0, 30),
    InvoiceNo:    "INV-001",
    RefNo:        refNo, // Estonian reference number
    Currency:     "EUR",
    Lines: []accounting.CreateInvoiceLineInput{
        {
            Code:        "SERVICE-001",
            Description: "Consulting services",
            Quantity:    decimal.NewFromInt(10),
            UnitPrice:   decimal.NewFromFloat(100.00),
            TaxID:       "vat-20",
            AccountCode: "3000",
        },
    },
})
```

### Managing Customers

```go
// Create a customer
customer, err := client.Customers.Create(ctx, accounting.CreateCustomerInput{
    Name:        "Acme Corp",
    Email:       "billing@acme.com",
    CountryCode: "EE",
    Currency:    "EUR",
})

// Find customer by email
customer, err := client.Customers.FindByEmail(ctx, "billing@acme.com")
```

## Features

- Customer management
- Invoice management
- Payment processing
- Purchase tracking
- Tax calculations
- Report generation
- Estonian reference number (viitenumber) generation using 3-7-1 algorithm
- Multi-provider support (SimplBooks, Merit)

## License

[Add your license here]
