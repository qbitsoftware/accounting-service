package smartaccounts

import (
	"bytes"
	"encoding/json"

	"github.com/shopspring/decimal"
)

// SmartAccounts dates are formatted "dd.MM.yyyy" and amounts use "." as the
// decimal separator. shopspring/decimal unmarshals JSON numbers and strings.

// flexString unmarshals either a JSON string or a JSON number into a string.
// SmartAccounts documents accounting/reference numbers as String but in
// practice can return them as bare JSON numbers (e.g. {"number": 42}). This
// type accepts both so the decoder doesn't break on either form. Outgoing
// requests still use plain `string` — flexString is read-side only.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	t := bytes.TrimSpace(b)
	if len(t) == 0 || string(t) == "null" {
		*f = ""
		return nil
	}
	if t[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	// Numeric (or boolean/other scalar) — store the raw JSON token.
	*f = flexString(t)
	return nil
}

func (f flexString) String() string { return string(f) }

// --- Shared ---

// PartnerRef is the nested client/vendor object embedded in invoice and
// payment GET responses (GET-only, per spec).
type PartnerRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	RegCode   string `json:"regCode"`
	VatNumber string `json:"vatNumber"`
}

// Address is the nested address object on clients/vendors.
type Address struct {
	Country    string `json:"country,omitempty"`
	County     string `json:"county,omitempty"`
	City       string `json:"city,omitempty"`
	Address1   string `json:"address1,omitempty"`
	Address2   string `json:"address2,omitempty"`
	PostalCode string `json:"postalCode,omitempty"`
}

// Contact is one entry in a client/vendor contacts array. Type is one of
// EMAIL, PHONE, FAX, WWW, SKYPE, OTHER, PERSON, POST_ADDRESS (case sensitive).
type Contact struct {
	Type        string `json:"type"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

// Contact type constants.
const (
	ContactEmail = "EMAIL"
	ContactPhone = "PHONE"
	ContactWWW   = "WWW"
)

// --- Client Invoices (purchasesales/clientinvoices) ---

// InvoiceType values. An empty/absent type is a regular invoice; "CRE" marks a
// credit invoice.
const InvoiceTypeCredit = "CRE"

// InvoiceRow is a sales/purchase invoice line as returned by the GET endpoints
// (read-only). Requests use InvoiceRowInput. Note: `omitempty` on a non-pointer
// decimal.Decimal is a no-op (the struct is never the empty value), so it is
// intentionally not used here — this is a decode target and the tags only need
// to name the JSON fields.
type InvoiceRow struct {
	Code         string          `json:"code"`
	Description  string          `json:"description"`
	Price        decimal.Decimal `json:"price"`
	Quantity     decimal.Decimal `json:"quantity"`
	Unit         string          `json:"unit"`
	Discount     decimal.Decimal `json:"discount"`
	Vat          decimal.Decimal `json:"vat"`
	VatPc        string          `json:"vatPc"`
	Sum          decimal.Decimal `json:"sum"`
	Order        int             `json:"order"`
	ObjectID     string          `json:"objectId"`
	AccountSales string          `json:"accountSales"`
}

// InvoiceItem is a client invoice as returned by clientinvoices:get and used,
// in part, for :add/:edit.
type InvoiceItem struct {
	ID       string      `json:"id"`
	ClientID string      `json:"clientId"`
	Client   *PartnerRef `json:"client,omitempty"`
	Type     string      `json:"type"`
	// BaseForCreditInvoiceID — on a credit invoice (type=CRE), the id of the
	// original invoice it credits. Present on the read side too.
	BaseForCreditInvoiceID string          `json:"baseForCreditInvoiceId,omitempty"`
	Date                   string          `json:"date"`
	DueDate                string          `json:"dueDate"`
	EntryDate              string          `json:"entryDate"`
	Number                 flexString      `json:"number"`
	InvoiceNumber          flexString      `json:"invoiceNumber"`
	ReferenceNumber        flexString      `json:"referenceNumber"`
	Currency               string          `json:"currency"`
	ExchangeRate           decimal.Decimal `json:"exchangeRate"`
	Amount                 decimal.Decimal `json:"amount"`
	RoundAmount            decimal.Decimal `json:"roundAmount"`
	VatAmount              decimal.Decimal `json:"vatAmount"`
	TotalAmount            decimal.Decimal `json:"totalAmount"`
	OutstandingAmount      decimal.Decimal `json:"outstandingAmount"`
	Rows                   []InvoiceRow    `json:"rows,omitempty"`
}

// InvoiceRowInput is an invoice line for :add/:edit requests. It deliberately
// omits computed/optional fields (sum, vat, order) so they are not sent as
// zero values — SmartAccounts computes them from price/quantity/vatPc.
type InvoiceRowInput struct {
	Code         string           `json:"code,omitempty"`
	Description  string           `json:"description,omitempty"`
	Price        decimal.Decimal  `json:"price"`
	Quantity     decimal.Decimal  `json:"quantity"`
	Unit         string           `json:"unit,omitempty"`
	VatPc        string           `json:"vatPc,omitempty"`
	Discount     *decimal.Decimal `json:"discount,omitempty"`
	AccountSales string           `json:"accountSales,omitempty"`
	ObjectID     string           `json:"objectId,omitempty"`
}

// CreateInvoiceRequest is the body for clientinvoices:add / :edit.
type CreateInvoiceRequest struct {
	ID       string `json:"id,omitempty"` // set for :edit
	ClientID string `json:"clientId,omitempty"`
	Type     string `json:"type,omitempty"` // "CRE" for credit invoices
	// BaseForCreditInvoiceID links a credit invoice (Type=CRE) to the SmartAccounts
	// id of the invoice it credits, so the credit offsets that invoice's balance.
	BaseForCreditInvoiceID string `json:"baseForCreditInvoiceId,omitempty"`
	Date                   string `json:"date"`
	// EntryDate is normally set by SmartAccounts to today's server time. It is
	// only explicitly set on credit invoices that credit a future-dated original,
	// to satisfy SA's "entry date must not be before initial invoice entry date"
	// check (CREDIT-INVOICE-ENTRY-EARLIER-THAN-BASE).
	EntryDate       string            `json:"entryDate,omitempty"`
	DueDate         string            `json:"dueDate,omitempty"`
	InvoiceNumber   string            `json:"invoiceNumber,omitempty"`
	ReferenceNumber string            `json:"referenceNumber,omitempty"`
	Currency        string            `json:"currency,omitempty"`
	TotalAmount     *decimal.Decimal  `json:"totalAmount,omitempty"`
	InvoiceNote     string            `json:"invoiceNote,omitempty"`
	Comment         string            `json:"comment,omitempty"`
	Rows            []InvoiceRowInput `json:"rows"`
}

// InvoiceResponse is the return value of clientinvoices:add / :edit.
type InvoiceResponse struct {
	InvoiceID       string          `json:"invoiceId"`
	ClientID        string          `json:"clientId"`
	Number          flexString      `json:"number"`
	InvoiceNumber   flexString      `json:"invoiceNumber"`
	ReferenceNumber flexString      `json:"referenceNumber"`
	Amount          decimal.Decimal `json:"amount"`
	VatAmount       decimal.Decimal `json:"vatAmount"`
	TotalAmount     decimal.Decimal `json:"totalAmount"`
	RoundAmount     decimal.Decimal `json:"roundAmount"`
	DueDate         string          `json:"dueDate"`
}

// PDFResponse is the base64 PDF payload from a :getpdf method.
type PDFResponse struct {
	FileName    string `json:"fileName"`
	FileContent string `json:"fileContent"` // base64
}

// --- Payments (purchasesales/payments) ---

// Partner type and account type constants for payments.
const (
	PartnerClient = "CLIENT"
	PartnerVendor = "VENDOR"
	AccountBank   = "BANK"
	AccountCash   = "CASH"
)

// Payment row types.
const (
	RowClientInvoice = "CLIENT_INVOICE"
	RowVendorInvoice = "VENDOR_INVOICE"
)

// PaymentRow links a payment to a document (e.g. a client invoice).
type PaymentRow struct {
	Description string          `json:"description,omitempty"`
	Amount      decimal.Decimal `json:"amount"`
	Type        string          `json:"type"`         // CLIENT_INVOICE, VENDOR_INVOICE, ...
	ID          string          `json:"id,omitempty"` // id of the linked document
}

// PaymentItem is a payment as returned by payments:get and used for :add.
type PaymentItem struct {
	ID          string          `json:"id"`
	Date        string          `json:"date"`
	Number      flexString      `json:"number"`
	Document    string          `json:"document"`
	PartnerType string          `json:"partnerType"`
	ClientID    string          `json:"clientId,omitempty"`
	VendorID    string          `json:"vendorId,omitempty"`
	Client      *PartnerRef     `json:"client,omitempty"`
	Vendor      *PartnerRef     `json:"vendor,omitempty"`
	AccountType string          `json:"accountType"`
	AccountName string          `json:"accountName"`
	Currency    string          `json:"currency"`
	Amount      decimal.Decimal `json:"amount"`
	Rows        []PaymentRow    `json:"rows,omitempty"`
}

// CreatePaymentRequest is the body for payments:add.
type CreatePaymentRequest struct {
	Date        string          `json:"date"`
	Document    string          `json:"document,omitempty"`
	PartnerType string          `json:"partnerType"`
	ClientID    string          `json:"clientId,omitempty"`
	VendorID    string          `json:"vendorId,omitempty"`
	AccountType string          `json:"accountType,omitempty"`
	AccountName string          `json:"accountName"`
	Currency    string          `json:"currency,omitempty"`
	Amount      decimal.Decimal `json:"amount"`
	Comment     string          `json:"comment,omitempty"`
	Rows        []PaymentRow    `json:"rows"`
}

// PaymentResponse is the return value of payments:add / :edit.
type PaymentResponse struct {
	PaymentID string          `json:"paymentId"`
	Number    flexString      `json:"number"`
	Amount    decimal.Decimal `json:"amount"`
}

// --- Clients (purchasesales/clients) ---

// ClientItem is a client (customer) record. Email/phone live in Contacts.
type ClientItem struct {
	ID              string    `json:"id"`
	Group           string    `json:"group,omitempty"`
	Name            string    `json:"name"`
	RegCode         string    `json:"regCode,omitempty"`
	VatNumber       string    `json:"vatNumber,omitempty"`
	BankAccount     string    `json:"bankAccount,omitempty"`
	ReferenceNumber string    `json:"referenceNumber,omitempty"`
	InvoiceDueDate  int       `json:"invoiceDueDate,omitempty"`
	Address         *Address  `json:"address,omitempty"`
	Contacts        []Contact `json:"contacts,omitempty"`
}

// CreateClientRequest is the body for clients:add / :edit.
type CreateClientRequest struct {
	ID             string    `json:"id,omitempty"` // set for :edit
	Name           string    `json:"name"`
	RegCode        string    `json:"regCode,omitempty"`
	VatNumber      string    `json:"vatNumber,omitempty"`
	InvoiceDueDate *int      `json:"invoiceDueDate,omitempty"`
	Address        *Address  `json:"address,omitempty"`
	Contacts       []Contact `json:"contacts,omitempty"`
}

// ClientResponse is the return value of clients:add / :edit.
type ClientResponse struct {
	ClientID        string `json:"clientId"`
	ReferenceNumber string `json:"referenceNumber"`
}

// --- Articles (purchasesales/articles) ---

// Article types (case sensitive).
const (
	ArticleProduct   = "PRODUCT"
	ArticleService   = "SERVICE"
	ArticleWarehouse = "WH"
)

// ArticleItem is an article (item) record.
type ArticleItem struct {
	Code            string          `json:"code"`
	Description     string          `json:"description"`
	Type            string          `json:"type"`
	Unit            string          `json:"unit,omitempty"`
	VatPc           string          `json:"vatPc,omitempty"`
	ActiveSales     bool            `json:"activeSales,omitempty"`
	ActivePurchase  bool            `json:"activePurchase,omitempty"`
	PriceSales      decimal.Decimal `json:"priceSales,omitempty"`
	PricePurchase   decimal.Decimal `json:"pricePurchase,omitempty"`
	AccountSales    string          `json:"accountSales,omitempty"`
	AccountPurchase string          `json:"accountPurchase,omitempty"`
}

// ArticleResponse is the return value of articles:add / :edit.
type ArticleResponse struct {
	Code string `json:"code"`
}

// --- Vendors (purchasesales/vendors) ---

// VendorItem is a vendor record.
type VendorItem struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	RegCode         string    `json:"regCode,omitempty"`
	VatNumber       string    `json:"vatNumber,omitempty"`
	ReferenceNumber string    `json:"referenceNumber,omitempty"`
	Address         *Address  `json:"address,omitempty"`
	Contacts        []Contact `json:"contacts,omitempty"`
}

// CreateVendorRequest is the body for vendors:add / :edit.
type CreateVendorRequest struct {
	ID        string    `json:"id,omitempty"`
	Name      string    `json:"name"`
	RegCode   string    `json:"regCode,omitempty"`
	VatNumber string    `json:"vatNumber,omitempty"`
	Address   *Address  `json:"address,omitempty"`
	Contacts  []Contact `json:"contacts,omitempty"`
}

// VendorResponse is the return value of vendors:add / :edit.
type VendorResponse struct {
	VendorID        string `json:"vendorId"`
	ReferenceNumber string `json:"referenceNumber"`
}

// --- Vendor Invoices (purchasesales/vendorinvoices) ---

// VendorInvoiceItem is a purchase invoice as returned by vendorinvoices:get.
type VendorInvoiceItem struct {
	ID                string          `json:"id"`
	VendorID          string          `json:"vendorId"`
	Vendor            *PartnerRef     `json:"vendor,omitempty"`
	Date              string          `json:"date"`
	DueDate           string          `json:"dueDate"`
	Number            flexString      `json:"number"`
	InvoiceNumber     flexString      `json:"invoiceNumber"`
	ReferenceNumber   flexString      `json:"referenceNumber"`
	Currency          string          `json:"currency"`
	Amount            decimal.Decimal `json:"amount"`
	VatAmount         decimal.Decimal `json:"vatAmount"`
	TotalAmount       decimal.Decimal `json:"totalAmount"`
	OutstandingAmount decimal.Decimal `json:"outstandingAmount"`
	Rows              []InvoiceRow    `json:"rows,omitempty"`
}

// CreateVendorInvoiceRequest is the body for vendorinvoices:add.
type CreateVendorInvoiceRequest struct {
	VendorID        string            `json:"vendorId,omitempty"`
	Date            string            `json:"date"`
	DueDate         string            `json:"dueDate,omitempty"`
	InvoiceNumber   string            `json:"invoiceNumber,omitempty"`
	ReferenceNumber string            `json:"referenceNumber,omitempty"`
	Currency        string            `json:"currency,omitempty"`
	IsCalculateVat  bool              `json:"isCalculateVat"`
	TotalAmount     *decimal.Decimal  `json:"totalAmount,omitempty"`
	Comment         string            `json:"comment,omitempty"`
	Rows            []InvoiceRowInput `json:"rows"`
}

// VendorInvoiceResponse is the return value of vendorinvoices:add / :edit.
type VendorInvoiceResponse struct {
	InvoiceID       string          `json:"invoiceId"`
	VendorID        string          `json:"vendorId"`
	Number          flexString      `json:"number"`
	InvoiceNumber   flexString      `json:"invoiceNumber"`
	ReferenceNumber flexString      `json:"referenceNumber"`
	Amount          decimal.Decimal `json:"amount"`
	VatAmount       decimal.Decimal `json:"vatAmount"`
	TotalAmount     decimal.Decimal `json:"totalAmount"`
}

// --- Settings: VatPcs / Accounts / Bank Accounts / Objects ---

// VatPc is a VAT-percentage register entry (settings/vatpcs).
type VatPc struct {
	VatPc         string          `json:"vatPc"`
	Pc            decimal.Decimal `json:"pc"`
	DescriptionEt string          `json:"descriptionEt"`
	DescriptionEn string          `json:"descriptionEn"`
	ActiveSales   bool            `json:"activeSales"`
}

// AccountItem is a general-ledger account (settings/accounts).
type AccountItem struct {
	Code          string `json:"code"`
	ID            string `json:"id"`
	Type          string `json:"type"` // ASSET, LIABILITY, INCOME, EXPENSE
	DescriptionEt string `json:"descriptionEt"`
	DescriptionEn string `json:"descriptionEn"`
}

// BankAccountItem is a bank account (settings/bankaccounts). Keyed by name.
type BankAccountItem struct {
	Name     string `json:"name"`
	Account  string `json:"account"`
	Currency string `json:"currency"`
	IBAN     string `json:"iban"`
	Swift    string `json:"swift"`
}

// ObjectItem is an accounting object/dimension (settings/objects).
type ObjectItem struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}
