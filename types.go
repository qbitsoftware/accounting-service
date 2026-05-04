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
	RefNoBase   string // Base for per-customer reference number generation
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

	// ExternalPayMode is the provider-specific payment-method identifier:
	// for Excellent Books this is the PayMode code (e.g. "P1", "K"); for
	// Merit it is the BankId GUID. Empty when the provider does not expose
	// per-receipt method information.
	ExternalPayMode string
	// ExternalBankName is the human-readable bank label, populated when the
	// provider exposes it directly. Merit returns this on getpayments;
	// Excellent Books does not.
	ExternalBankName string
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

// Bank represents a bank account configured in the accounting system. Returned
// by ListBanks. Merit's /getbanks exposes Name, IBAN, BankID; Excellent Books
// has no equivalent register and ListBanks returns nil there.
type Bank struct {
	ID           string
	Name         string
	Description  string
	IBAN         string
	AccountCode  string
	CurrencyCode string
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
	ID            string          `json:"id"`
	Code          string          `json:"code"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Type          ItemType        `json:"type"`
	UnitOfMeasure string          `json:"unit_of_measure"`
	SalesPrice    decimal.Decimal `json:"sales_price"`
	TaxID         string          `json:"tax_id"`
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

// Dimension represents a Merit dimension entry (project, cost center, or department).
type Dimension struct {
	Code    string
	Name    string
	DimID   int    // Merit dimension type ID (from v2/getdimensions)
	ValueID string // Merit dimension value GUID (from v2/getdimensions)
}

// DimensionList holds all available dimension reference data from Merit.
type DimensionList struct {
	Projects    []Dimension
	CostCenters []Dimension
	Departments []Dimension
}
