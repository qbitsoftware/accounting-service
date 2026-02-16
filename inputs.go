package accounting

import (
	"time"

	"github.com/shopspring/decimal"
)

type CreateInvoiceInput struct {
	CustomerID          string
	CustomerName        string
	CustomerRegNo       string
	CustomerEmail       string
	CustomerAddress     string
	CustomerCountryCode string
	DocDate             time.Time
	DueDate             time.Time
	InvoiceNo           string
	RefNo               string
	Currency            string
	Lines               []CreateInvoiceLineInput
	TotalAmount         decimal.Decimal // Total invoice amount (for validation)
	Comment             string
	FooterComment       string
}

type CreateInvoiceLineInput struct {
	Code        string
	Description string
	Quantity    decimal.Decimal
	UnitPrice   decimal.Decimal
	TaxID       string
	AccountCode string
	Type        *int   // Item type: 1=product, 2=package, 3=service (default: 3)
	UOMName     string // Unit of measure (e.g., "pcs", "hrs", "kg")
}

type CreateCustomerInput struct {
	Name        string
	RegNo       string
	VATRegNo    string
	Email       string
	Phone       string
	Address     string
	City        string
	County      string
	PostalCode  string
	CountryCode string
	Currency    string
	PaymentDays *int
	Contact     string
}

type UpdateCustomerInput struct {
	ID          string
	Name        *string
	Email       *string
	Phone       *string
	Address     *string
	City        *string
	PostalCode  *string
	CountryCode *string
	RegNo       *string
	VATRegNo    *string
}

type CreatePaymentInput struct {
	CustomerName string
	InvoiceNo    string
	PaymentDate  time.Time
	Amount       decimal.Decimal
	Currency     string
	BankID       string
}

type ListInvoicesInput struct {
	PeriodStart time.Time
	PeriodEnd   time.Time
}

type ListPaymentsInput struct {
	PeriodStart time.Time
	PeriodEnd   time.Time
}

type ListCustomersInput struct{}

type CreateItemInput struct {
	Code                string
	Description         string
	Type                ItemType
	UnitOfMeasure       string
	SalesPrice          decimal.Decimal
	TaxID               string
	SalesAccountCode    string
	PurchaseAccountCode string
}

type UpdateItemInput struct {
	ID          string
	Code        *string
	Description *string
	SalesPrice  *decimal.Decimal
	TaxID       *string
}

type ListItemsInput struct {
	Code        string
	Description string
	Type        ItemType
}

type CreateCreditNoteInput struct {
	CustomerID          string
	CustomerName        string
	CustomerRegNo       string
	CustomerEmail       string
	CustomerAddress     string
	CustomerCountryCode string
	DocDate             time.Time
	DueDate             time.Time
	InvoiceNo           string
	RefNo               string
	Currency            string
	Lines               []CreateInvoiceLineInput
	Comment             string
	FooterComment       string
	OriginalInvoiceNo   string
}

type CreatePurchaseInput struct {
	VendorID          string
	VendorName        string
	VendorRegNo       string
	VendorEmail       string
	VendorAddress     string
	VendorCountryCode string
	DocDate           time.Time
	DueDate           time.Time
	BillNo            string
	RefNo             string
	Currency          string
	Lines             []CreateInvoiceLineInput
	Comment           string
	FooterComment     string
}

type ListPurchasesInput struct {
	PeriodStart time.Time
	PeriodEnd   time.Time
}
