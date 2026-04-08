package directo

import "fmt"

// APIError represents an error response from the Directo API.
type APIError struct {
	StatusCode int
	Message    string
	Source     string // "rest" or "xml"
}

func (e *APIError) Error() string {
	return fmt.Sprintf("directo %s api: status %d: %s", e.Source, e.StatusCode, e.Message)
}

// --- REST API response types (JSON) ---

// CustomerREST represents a customer from the REST API.
type CustomerREST struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	RegNo       string `json:"reg_no"`
	VATNo       string `json:"vat_no"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Address     string `json:"address"`
	City        string `json:"city"`
	County      string `json:"county"`
	PostalCode  string `json:"postal_code"`
	Country     string `json:"country"`
	Currency    string `json:"currency"`
	PaymentDays int    `json:"payment_days"`
	Contact     string `json:"contact"`
	HomePage    string `json:"homepage"`
	Timestamp   string `json:"ts"`
}

// InvoiceREST represents a sales invoice from the REST API.
type InvoiceREST struct {
	Number       string `json:"number"`
	CustomerCode string `json:"customer_code"`
	CustomerName string `json:"customer_name"`
	Date         string `json:"date"`
	Deadline     string `json:"deadline"`
	Total        string `json:"total"`
	TotalTax     string `json:"total_tax"`
	PaidAmount   string `json:"paid_amount"`
	Currency     string `json:"currency"`
	Status       string `json:"status"`
	RefNo        string `json:"ref_no"`
	Comment      string `json:"comment"`
	Confirmed    string `json:"confirmed"`
	Timestamp    string `json:"ts"`
}

// InvoiceLineREST represents an invoice line from the REST API.
type InvoiceLineREST struct {
	Item        string `json:"row_item"`
	Description string `json:"row_description"`
	Quantity    string `json:"row_quantity"`
	Price       string `json:"row_price"`
	Discount    string `json:"row_discount"`
	Tax         string `json:"row_tax"`
	Account     string `json:"row_account"`
	Total       string `json:"row_total"`
}

// ItemREST represents an item/article from the REST API.
type ItemREST struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Class       string `json:"class"`
	Status      string `json:"status"`
	Unit        string `json:"unit"`
	Price       string `json:"price"`
	Cost        string `json:"cost"`
	Country     string `json:"country"`
	Barcode     string `json:"barcode"`
	Description string `json:"description"`
	Timestamp   string `json:"ts"`
}

// AccountREST represents a GL account from the REST API.
type AccountREST struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

// TaxREST represents a tax/VAT code.
// Directo may not expose taxes via REST API, so this might come from XML.
type TaxREST struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Pct  string `json:"pct"`
}

// ReceiptREST represents a receipt/payment from the REST API.
type ReceiptREST struct {
	Number       string `json:"number"`
	Date         string `json:"date"`
	Amount       string `json:"amount"`
	Currency     string `json:"currency"`
	CustomerCode string `json:"customer_code"`
	CustomerName string `json:"customer_name"`
	InvoiceNo    string `json:"invoice_no"`
	Timestamp    string `json:"ts"`
}

// ObjectREST represents an object/dimension from the REST API.
type ObjectREST struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

// ProjectREST represents a project from the REST API.
type ProjectREST struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// --- List params ---

// InvoiceListParams specifies parameters for listing invoices via REST.
type InvoiceListParams struct {
	DateFrom string // Filter: date=>YYYY-MM-DDTHH:mm:ss
	DateTo   string // Filter: date=<YYYY-MM-DDTHH:mm:ss
	TSFrom   string // Filter: ts=>timestamp (for incremental sync)
	Status   string // Filter: status=X
}

// ItemListParams specifies parameters for listing items via REST.
type ItemListParams struct {
	Code   string
	Class  string
	Status string
	TSFrom string
}

// PaymentListParams specifies parameters for listing receipts via REST.
type PaymentListParams struct {
	DateFrom string
	DateTo   string
	TSFrom   string
}
