package accounting

import (
	"time"

	"github.com/shopspring/decimal"
)

type InvoiceStatus string

const (
	InvoiceStatusUnpaid  InvoiceStatus = "unpaid"
	InvoiceStatusPartial InvoiceStatus = "partial"
	InvoiceStatusPaid    InvoiceStatus = "paid"
)

type PaymentDirection string

const (
	PaymentDirectionCustomer     PaymentDirection = "customer"
	PaymentDirectionVendor       PaymentDirection = "vendor"
	PaymentDirectionOtherIncome  PaymentDirection = "other_income"
	PaymentDirectionOtherExpense PaymentDirection = "other_expense"
)

type Invoice struct {
	ID           string
	Number       string
	CustomerName string
	CustomerID   string
	DocDate      time.Time
	DueDate      time.Time
	TotalAmount  decimal.Decimal
	TaxAmount    decimal.Decimal
	PaidAmount   decimal.Decimal
	Currency     string
	Paid         bool
	Status       InvoiceStatus
	ReferenceNo  string
	Lines        []InvoiceLine
	Payments     []InvoicePayment
}

type InvoiceLine struct {
	ID            string
	Description   string
	Quantity      decimal.Decimal
	UnitPrice     decimal.Decimal
	TaxID         string
	TaxName       string
	TaxPct        decimal.Decimal
	AmountExclVat decimal.Decimal
	AmountInclVat decimal.Decimal
	VatAmount     decimal.Decimal
	AccountCode   string
}

type InvoicePayment struct {
	Date      time.Time
	Amount    decimal.Decimal
	Method    string
	PaymentID string
}

// InvoicePDF represents a PDF document for an invoice.
type InvoicePDF struct {
	FileName    string // Name of the PDF file
	FileContent []byte // Raw PDF bytes (decoded from base64)
}

type Customer struct {
	ID          string
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
	PaymentDays int
	Contact     string
	HomePage    string
}

type Payment struct {
	ID              string
	DocumentNo      string
	DocumentDate    time.Time
	Amount          decimal.Decimal
	Currency        string
	Direction       PaymentDirection
	CounterPartID   string
	CounterPartName string
	InvoiceLinks    []PaymentInvoiceLink
}

type PaymentInvoiceLink struct {
	InvoiceID string
	InvoiceNo string
	Amount    decimal.Decimal
}

type Tax struct {
	ID   string
	Code string
	Name string
	Pct  decimal.Decimal
}

type Account struct {
	ID     string
	Code   string
	Name   string
	Active bool
}

type CustomerDebt struct {
	CustomerName string
	CustomerID   string
	DocType      string
	DocDate      time.Time
	DocNo        string
	DueDate      time.Time
	TotalAmount  decimal.Decimal
	PaidAmount   decimal.Decimal
	UnpaidAmount decimal.Decimal
	Currency     string
}

type ItemType string

const (
	ItemTypeStock   ItemType = "stock"
	ItemTypeService ItemType = "service"
	ItemTypeItem    ItemType = "item"
)

type Item struct {
	ID             string
	Code           string
	Name           string
	Description    string
	Type           ItemType
	UnitOfMeasure  string
	SalesPrice     decimal.Decimal
	TaxID          string
}

type PurchaseInvoice struct {
	ID          string
	Number      string
	VendorName  string
	VendorID    string
	DocDate     time.Time
	DueDate     time.Time
	TotalAmount decimal.Decimal
	TaxAmount   decimal.Decimal
	PaidAmount  decimal.Decimal
	Currency    string
	Paid        bool
	Status      InvoiceStatus
	ReferenceNo string
}

type BatchResult struct {
	Invoice *Invoice
	Err     error
	Index   int
}
