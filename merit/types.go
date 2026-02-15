package merit

import (
	"time"

	"github.com/shopspring/decimal"
)

// Accounting document types.
const (
	DocInvoice      = 1
	DocReceipt      = 2
	DocReceipt2     = 3
	DocNoDoc        = 4
	DocCredit       = 5
	DocPrepInvoice  = 6
	DocFinCharge    = 7
	DocDelivOrder   = 8
	DocGroupInvoice = 9
)

// Item types.
const (
	ItemTypeStock   = 1
	ItemTypeService = 2
	ItemTypeItem    = 3
)

// Item usage types.
const (
	ItemUsageSales    = 1
	ItemUsagePurchase = 2
	ItemUsageBoth     = 3
)

// E-Invoice operator types.
const (
	EInvNotExist    = 1
	EInvOmniva      = 2
	EInvBankFull    = 3
	EInvBankLimited = 4
)

// Payment direction types.
const (
	DirectionCustomers     = 1
	DirectionVendors       = 2
	DirectionOtherIncome   = 3
	DirectionOtherExpenses = 4
)

// CounterPart types.
const (
	CounterPartCustomer = 2
	CounterPartVendor   = 3
)

// --- Sales Invoices ---

// ListInvoicesParams specifies parameters for listing sales invoices.
type ListInvoicesParams struct {
	PeriodStart time.Time `json:"PeriodStart"`
	PeriodEnd   time.Time `json:"PeriodEnd"`
	UnPaid      bool      `json:"UnPaid,omitempty"`
	DateType    *int      `json:"DateType,omitempty"` // 0=document date, 1=changed date
}

// InvoiceListItem represents a sales invoice in list results.
type InvoiceListItem struct {
	SIHId           string          `json:"SIHId"`
	DepartmentCode  string          `json:"DepartmentCode"`
	DepartmentName  string          `json:"DepartmentName"`
	InvoiceNo       string          `json:"InvoiceNo"`
	DocumentDate    string          `json:"DocumentDate"`
	TransactionDate string          `json:"TransactionDate"`
	CustomerName    string          `json:"CustomerName"`
	CustomerRegNo   string          `json:"CustomerRegNo"`
	CustomerID      string          `json:"CustomerId"`
	HComment        string          `json:"HComment"`
	FComment        string          `json:"FComment"`
	DueDate         string          `json:"DueDate"`
	CurrencyCode    string          `json:"CurrencyCode"`
	CurrencyRate    decimal.Decimal `json:"CurrencyRate"`
	TaxAmount       decimal.Decimal `json:"TaxAmount"`
	RoundingAmount  decimal.Decimal `json:"RoundingAmount"`
	TotalAmount     decimal.Decimal `json:"TotalAmount"`
	ProfitAmount    decimal.Decimal `json:"ProfitAmount"`
	TotalSum        decimal.Decimal `json:"TotalSum"`
	UserName        string          `json:"UserName"`
	ReferenceNo     string          `json:"ReferenceNo"`
	PriceInclVat    bool            `json:"PriceInclVat"`
	VatRegNo        string          `json:"VatRegNo"`
	PaidAmount      decimal.Decimal `json:"PaidAmount"`
	EInvSent        bool            `json:"EInvSent"`
	EmailSent       string          `json:"EmailSent"`
	Paid            bool            `json:"Paid"`
	ChangedDate     string          `json:"ChangedDate"`
	AccountingDoc   int             `json:"AccountingDoc"`
	BatchInfo       string          `json:"BatchInfo"`
}

// GetInvoiceParams specifies parameters for getting invoice details.
type GetInvoiceParams struct {
	ID            string `json:"Id"`
	AddAttachment bool   `json:"AddAttachment,omitempty"`
}

// InvoiceDetail represents detailed invoice information.
type InvoiceDetail struct {
	SIHId           string             `json:"SIHId"`
	DepartmentCode  string             `json:"DepartmentCode"`
	DepartmentName  string             `json:"DepartmentName"`
	ProjectCode     string             `json:"ProjectCode"`
	ProjectName     string             `json:"ProjectName"`
	BatchInfo       string             `json:"BatchInfo"`
	InvoiceNo       string             `json:"InvoiceNo"`
	DocumentDate    string             `json:"DocumentDate"`
	TransactionDate string             `json:"TransactionDate"`
	CustomerID      string             `json:"CustomerId"`
	CustomerName    string             `json:"CustomerName"`
	CustomerRegNo   string             `json:"CustomerRegNo"`
	HComment        string             `json:"HComment"`
	FComment        string             `json:"FComment"`
	DueDate         string             `json:"DueDate"`
	CurrencyCode    string             `json:"CurrencyCode"`
	CurrencyRate    decimal.Decimal    `json:"CurrencyRate"`
	TaxAmount       decimal.Decimal    `json:"TaxAmount"`
	RoundingAmount  decimal.Decimal    `json:"RoundingAmount"`
	TotalAmount     decimal.Decimal    `json:"TotalAmount"`
	ProfitAmount    decimal.Decimal    `json:"ProfitAmount"`
	TotalSum        decimal.Decimal    `json:"TotalSum"`
	UserName        string             `json:"UserName"`
	ReferenceNo     string             `json:"ReferenceNo"`
	PriceInclVat    bool               `json:"PriceInclVat"`
	VatRegNo        string             `json:"VatRegNo"`
	PaidAmount      decimal.Decimal    `json:"PaidAmount"`
	EInvSent        bool               `json:"EInvSent"`
	EmailSent       string             `json:"EmailSent"`
	EInvOperator    int                `json:"EInvOperator"`
	ContractNo      string             `json:"ContractNo"`
	Paid            bool               `json:"Paid"`
	Contact         string             `json:"Contact"`
	Dimensions      []DimensionRef     `json:"Dimensions"`
	Lines           []InvoiceDetailRow `json:"Lines"`
	Payments        []PaymentInfo      `json:"Payments"`
	Attachment      *Attachment        `json:"Attachment"`
}

// InvoiceDetailRow represents a line item in invoice details.
type InvoiceDetailRow struct {
	SILId          string          `json:"SILId"`
	ArticleCode    string          `json:"ArticleCode"`
	LocationCode   string          `json:"LocationCode"`
	Quantity       decimal.Decimal `json:"Quantity"`
	Price          decimal.Decimal `json:"Price"`
	TaxID          string          `json:"TaxId"`
	TaxName        string          `json:"TaxName"`
	TaxPct         decimal.Decimal `json:"TaxPct"`
	AmountExclVat  decimal.Decimal `json:"AmountExclVat"`
	AmountInclVat  decimal.Decimal `json:"AmountInclVat"`
	VatAmount      decimal.Decimal `json:"VatAmount"`
	AccountCode    string          `json:"AccountCode"`
	DepartmentCode string          `json:"DepartmentCode"`
	DepartmentName string          `json:"DepartmentName"`
	ItemCostAmount decimal.Decimal `json:"ItemCostAmount"`
	ProfitAmount   decimal.Decimal `json:"ProfitAmount"`
	DiscountPct    decimal.Decimal `json:"DiscountPct"`
	DiscountAmount decimal.Decimal `json:"DiscountAmount"`
	Description    string          `json:"Description"`
	UOMName        string          `json:"UOMName"`
	FixAsset       bool            `json:"FixAsset"`
}

// PaymentInfo represents payment information on an invoice.
type PaymentInfo struct {
	PaymDate      string          `json:"PaymDate"`
	Amount        decimal.Decimal `json:"Amount"`
	PaymentMethod string          `json:"PaymentMethod"`
	PaymentID     string          `json:"PaymentId"`
}

// Attachment represents a PDF attachment.
type Attachment struct {
	FileName    string `json:"FileName"`
	FileContent string `json:"FileContent"`
}

// CreateInvoiceRequest represents the request body for creating a sales invoice.
type CreateInvoiceRequest struct {
	Customer        CustomerRef      `json:"Customer"`
	AccountingDoc   int              `json:"AccountingDoc"`
	DocDate         string           `json:"DocDate,omitempty"`
	DueDate         string           `json:"DueDate,omitempty"`
	TransactionDate string           `json:"TransactionDate,omitempty"`
	InvoiceNo       string           `json:"InvoiceNo"`
	RefNo           string           `json:"RefNo,omitempty"`
	CurrencyCode    string           `json:"CurrencyCode,omitempty"`
	CurrencyRate    decimal.Decimal  `json:"CurrencyRate,omitempty"`
	DepartmentCode  string           `json:"DepartmentCode,omitempty"`
	Dimensions      []DimensionRef   `json:"Dimensions,omitempty"`
	InvoiceRow      []InvoiceRow     `json:"InvoiceRow"`
	TaxAmount       []TaxAmountEntry `json:"TaxAmount"`
	RoundingAmount  decimal.Decimal  `json:"RoundingAmount,omitempty"`
	TotalAmount     decimal.Decimal  `json:"TotalAmount,omitempty"`
	Payment         *PaymentEntry    `json:"Payment,omitempty"`
	Hcomment        string           `json:"Hcomment,omitempty"`
	Fcomment        string           `json:"Fcomment,omitempty"`
	ContractNo      string           `json:"ContractNo,omitempty"`
	PDF             string           `json:"PDF,omitempty"`
	FileName        string           `json:"FileName,omitempty"`
	Payer           *PayerRef        `json:"Payer,omitempty"`
	ReserveItems    bool             `json:"ReserveItems,omitempty"`
}

// CreateInvoiceResponse represents the response from creating a sales invoice.
type CreateInvoiceResponse struct {
	CustomerID  string `json:"CustomerId"`
	InvoiceID   string `json:"InvoiceId"`
	InvoiceNo   string `json:"InvoiceNo"`
	RefNo       string `json:"RefNo"`
	NewCustomer bool   `json:"NewCustomer"`
}

// GetInvoicePDFParams specifies parameters for getting an invoice PDF.
type GetInvoicePDFParams struct {
	ID        string `json:"Id"`
	DelivNote bool   `json:"DelivNote,omitempty"` // If true, returns delivery note without prices
}

// DeleteInvoiceParams specifies parameters for deleting a sales invoice.
type DeleteInvoiceParams struct {
	ID string `json:"Id"`
}

// --- Purchase Invoices ---

// ListPurchasesParams specifies parameters for listing purchase invoices.
type ListPurchasesParams struct {
	PeriodStart time.Time `json:"PeriodStart"`
	PeriodEnd   time.Time `json:"PeriodEnd"`
	DateType    *int      `json:"DateType,omitempty"` // 0=document date, 1=changed date
}

// PurchaseListItem represents a purchase invoice in list results.
type PurchaseListItem struct {
	PIHId          string          `json:"PIHId"`
	DepartmentCode string          `json:"DepartmentCode"`
	DepartmentName string          `json:"DepartmentName"`
	BatchInfo      string          `json:"BatchInfo"`
	BillNo         string          `json:"BillNo"`
	DocumentDate   string          `json:"DocumentDate"`
	TransactionDate string         `json:"TransactionDate"`
	DueDate        string          `json:"DueDate"`
	VendorID       string          `json:"VendorId"`
	VendorName     string          `json:"VendorName"`
	VendorRegNo    string          `json:"VendorRegNo"`
	ReferenceNo    string          `json:"ReferenceNo"`
	CurrencyCode   string          `json:"CurrencyCode"`
	CurrencyRate   decimal.Decimal `json:"CurrencyRate"`
	TaxAmount      decimal.Decimal `json:"TaxAmount"`
	RoundingAmount decimal.Decimal `json:"RoundingAmount"`
	TotalAmount    decimal.Decimal `json:"TotalAmount"`
	ProfitAmount   decimal.Decimal `json:"ProfitAmount"`
	TotalSum       decimal.Decimal `json:"TotalSum"`
	PriceInclVat   bool            `json:"PriceInclVat"`
	PaidAmount     decimal.Decimal `json:"PaidAmount"`
	FileExists     bool            `json:"FileExists"`
	Paid           bool            `json:"Paid"`
	ChangedDate    string          `json:"ChangedDate"`
}

// CreatePurchaseRequest represents the request body for creating a purchase invoice.
type CreatePurchaseRequest struct {
	Vendor          VendorRef        `json:"Vendor"`
	ExpenseClaim    bool             `json:"ExpenseClaim,omitempty"`
	DocDate         string           `json:"DocDate"`
	DueDate         string           `json:"DueDate"`
	TransactionDate string           `json:"TransactionDate"`
	BillNo          string           `json:"BillNo"`
	RefNo           string           `json:"RefNo,omitempty"`
	BankAccount     string           `json:"BankAccount,omitempty"`
	CurrencyCode    string           `json:"CurrencyCode,omitempty"`
	CurrencyRate    decimal.Decimal  `json:"CurrencyRate,omitempty"`
	DepartmentCode  string           `json:"DepartmentCode,omitempty"`
	Dimensions      []DimensionRef   `json:"Dimensions,omitempty"`
	InvoiceRow      []InvoiceRow     `json:"InvoiceRow"`
	TaxAmount       []TaxAmountEntry `json:"TaxAmount"`
	RoundingAmount  decimal.Decimal  `json:"RoundingAmount,omitempty"`
	TotalAmount     decimal.Decimal  `json:"TotalAmount,omitempty"`
	Payment         *PaymentEntry    `json:"Payment,omitempty"`
	Hcomment        string           `json:"Hcomment,omitempty"`
	Fcomment        string           `json:"Fcomment,omitempty"`
	AttachmentObj   *Attachment      `json:"Attachment,omitempty"`
}

// CreatePurchaseResponse represents the response from creating a purchase invoice.
type CreatePurchaseResponse struct {
	VendorID  string `json:"VendorId"`
	BillID    string `json:"BillId"`
	BillNo    string `json:"BillNo"`
	RefNo     string `json:"RefNo"`
	BatchInfo string `json:"BatcInfo"`
}

// DeletePurchaseParams specifies parameters for deleting a purchase invoice.
type DeletePurchaseParams struct {
	ID string `json:"Id"`
}

// --- Shared sub-types ---

// CustomerRef identifies a customer in invoice creation requests.
type CustomerRef struct {
	ID              string          `json:"Id,omitempty"`
	Name            string          `json:"Name,omitempty"`
	RegNo           string          `json:"RegNo,omitempty"`
	NotTDCustomer   *bool           `json:"NotTDCustomer,omitempty"`
	VatRegNo        string          `json:"VatRegNo,omitempty"`
	CurrencyCode    string          `json:"CurrencyCode,omitempty"`
	PaymentDeadLine *int            `json:"PaymentDeadLine,omitempty"`
	OverDueCharge   decimal.Decimal `json:"OverDueCharge,omitempty"`
	RefNoBase       string          `json:"RefNoBase,omitempty"`
	Address         string          `json:"Address,omitempty"`
	CountryCode     string          `json:"CountryCode,omitempty"`
	County          string          `json:"County,omitempty"`
	City            string          `json:"City,omitempty"`
	PostalCode      string          `json:"PostalCode,omitempty"`
	PhoneNo         string          `json:"PhoneNo,omitempty"`
	PhoneNo2        string          `json:"PhoneNo2,omitempty"`
	HomePage        string          `json:"HomePage,omitempty"`
	Email           string          `json:"Email,omitempty"`
	SalesInvLang    string          `json:"SalesInvLang,omitempty"`
	Contact         string          `json:"Contact,omitempty"`
	GLNCode         string          `json:"GLNCode,omitempty"`
	PartyCode       string          `json:"PartyCode,omitempty"`
	EInvOperator    *int            `json:"EInvOperator,omitempty"`
	EInvPaymId      string          `json:"EInvPaymId,omitempty"`
	BankAccount     string          `json:"BankAccount,omitempty"`
	Dimensions      []DimensionRef  `json:"Dimensions,omitempty"`
	CustGrCode      string          `json:"CustGrCode,omitempty"`
	ShowBalance     *bool           `json:"ShowBalance,omitempty"`
	ApixEInv        string          `json:"ApixEInv,omitempty"`
	GroupInv        *bool           `json:"GroupInv,omitempty"`
}

// PayerRef identifies a payer (if different from the customer) in invoice creation.
type PayerRef = CustomerRef

// VendorRef identifies a vendor in purchase invoice creation requests.
type VendorRef struct {
	ID              string          `json:"Id,omitempty"`
	Name            string          `json:"Name,omitempty"`
	RegNo           string          `json:"RegNo,omitempty"`
	VatAccountable  *bool           `json:"VatAccountable,omitempty"`
	VatRegNo        string          `json:"VatRegNo,omitempty"`
	CurrencyCode    string          `json:"CurrencyCode,omitempty"`
	PaymentDeadLine *int            `json:"PaymentDeadLine,omitempty"`
	OverDueCharge   decimal.Decimal `json:"OverDueCharge,omitempty"`
	Address         string          `json:"Address,omitempty"`
	City            string          `json:"City,omitempty"`
	County          string          `json:"County,omitempty"`
	PostalCode      string          `json:"PostalCode,omitempty"`
	CountryCode     string          `json:"CountryCode,omitempty"`
	PhoneNo         string          `json:"PhoneNo,omitempty"`
	PhoneNo2        string          `json:"PhoneNo2,omitempty"`
	HomePage        string          `json:"HomePage,omitempty"`
	Email           string          `json:"Email,omitempty"`
}

// InvoiceRow represents a line item in an invoice.
type InvoiceRow struct {
	Item            ItemRef         `json:"Item"`
	Quantity        decimal.Decimal `json:"Quantity,omitempty"`
	Price           decimal.Decimal `json:"Price,omitempty"`
	DiscountPct     decimal.Decimal `json:"DiscountPct,omitempty"`
	DiscountAmount  decimal.Decimal `json:"DiscountAmount,omitempty"`
	TaxID           string          `json:"TaxId"`
	LocationCode    string          `json:"LocationCode,omitempty"`
	DepartmentCode  string          `json:"DepartmentCode,omitempty"`
	GLAccountCode   string          `json:"GLAccountCode,omitempty"`
	Dimensions      []DimensionRef  `json:"Dimensions,omitempty"`
	ItemCostAmount  decimal.Decimal `json:"ItemCostAmount,omitempty"`
	ProjectCode     string          `json:"ProjectCode,omitempty"`
	CostCenterCode  string          `json:"CostCenterCode,omitempty"`
	VatDate         string          `json:"VatDate,omitempty"`
}

// ItemRef identifies an item/product in an invoice row.
type ItemRef struct {
	Code           string `json:"Code"`
	Description    string `json:"Description"`
	Type           *int   `json:"Type,omitempty"`
	UOMName        string `json:"UOMName,omitempty"`
	DefLocationCode string `json:"DefLocationCode,omitempty"`
	GTUCode        *int   `json:"GTUCode,omitempty"`
	SalesAccCode   string `json:"SalesAccCode,omitempty"`
	PurchaseAccCode string `json:"PuchaseAccCode,omitempty"` // Merit API uses "Puchase" (sic)
	InventoryAccCode string `json:"InventoryAccCode,omitempty"`
	CostAccCode     string `json:"CostAccCode,omitempty"`
}

// TaxAmountEntry represents a tax amount in an invoice.
type TaxAmountEntry struct {
	TaxID  string          `json:"TaxId"`
	Amount decimal.Decimal `json:"Amount,omitempty"`
}

// PaymentEntry represents a payment attached to an invoice.
type PaymentEntry struct {
	PaymentMethod string          `json:"PaymentMethod,omitempty"`
	PaidAmount    decimal.Decimal `json:"PaidAmount,omitempty"`
	PaymDate      string          `json:"PaymDate,omitempty"`
}

// DimensionRef represents a dimension reference.
type DimensionRef struct {
	DimID      *int   `json:"DimId,omitempty"`
	DimValueID string `json:"DimValueId,omitempty"`
	DimCode    string `json:"DimCode,omitempty"`
}

// --- Customers ---

// ListCustomersParams specifies parameters for listing customers.
type ListCustomersParams struct {
	ID           string `json:"Id,omitempty"`
	RegNo        string `json:"RegNo,omitempty"`
	VatRegNo     string `json:"VatRegNo,omitempty"`
	Name         string `json:"Name,omitempty"`
	WithComments bool   `json:"WithComments,omitempty"`
	CommentsFrom string `json:"CommentsFrom,omitempty"`
	ChangedDate  string `json:"ChangedDate,omitempty"`
}

// CustomerListItem represents a customer in list results.
type CustomerListItem struct {
	CustomerID        string          `json:"CustomerId"`
	Name              string          `json:"Name"`
	RegNo             string          `json:"RegNo"`
	Contact           string          `json:"Contact"`
	PhoneNo           string          `json:"PhoneNo"`
	PhoneNo2          string          `json:"PhoneNo2"`
	Address           string          `json:"Address"`
	City              string          `json:"City"`
	County            string          `json:"County"`
	PostalCode        string          `json:"PostalCode"`
	CountryName       string          `json:"CountryName"`
	CountryCode       string          `json:"CountryCode"`
	FaxNo             string          `json:"FaxNo"`
	Email             string          `json:"Email"`
	HomePage          string          `json:"HomePage"`
	CurrencyCode      string          `json:"CurrencyCode"`
	CustomerGroupID   string          `json:"CustomerGroupId"`
	CustomerGroupName string          `json:"CustomerGroupName"`
	PaymentDeadLine   int             `json:"PaymentDeadLine"`
	OverdueCharge     decimal.Decimal `json:"OverdueCharge"`
	VatRegNo          string          `json:"VatRegNo"`
	NotTDCustomer     bool            `json:"NotTDCustomer"`
	BankName          string          `json:"BankName"`
	BankAccount       string          `json:"BankAccount"`
	SalesInvLang      string          `json:"SalesInvLang"`
	RefNoBase         string          `json:"RefNoBase"`
	Dimensions        []DimensionRef  `json:"Dimensions"`
	ChangedDate       string          `json:"ChangedDate"`
	GLNCode           string          `json:"GLNCode"`
	PartyCode         string          `json:"PartyCode"`
}

// CreateCustomerRequest represents the request body for creating a customer.
type CreateCustomerRequest struct {
	ID              string          `json:"Id,omitempty"`
	Name            string          `json:"Name"`
	RegNo           string          `json:"RegNo,omitempty"`
	NotTDCustomer   bool            `json:"NotTDCustomer"`
	VatRegNo        string          `json:"VatRegNo,omitempty"`
	CurrencyCode    string          `json:"CurrencyCode,omitempty"`
	PaymentDeadLine *int            `json:"PaymentDeadLine,omitempty"`
	OverDueCharge   decimal.Decimal `json:"OverDueCharge,omitempty"`
	RefNoBase       string          `json:"RefNoBase,omitempty"`
	Address         string          `json:"Address,omitempty"`
	CountryCode     string          `json:"CountryCode,omitempty"`
	County          string          `json:"County,omitempty"`
	City            string          `json:"City,omitempty"`
	PostalCode      string          `json:"PostalCode,omitempty"`
	PhoneNo         string          `json:"PhoneNo,omitempty"`
	PhoneNo2        string          `json:"PhoneNo2,omitempty"`
	HomePage        string          `json:"HomePage,omitempty"`
	Email           string          `json:"Email,omitempty"`
	SalesInvLang    string          `json:"SalesInvLang,omitempty"`
	Contact         string          `json:"Contact,omitempty"`
	GLNCode         string          `json:"GLNCode,omitempty"`
	PartyCode       string          `json:"PartyCode,omitempty"`
	EInvOperator    *int            `json:"EInvOperator,omitempty"`
	EInvPaymId      string          `json:"EInvPaymId,omitempty"`
	BankAccount     string          `json:"BankAccount,omitempty"`
	CustGrCode      string          `json:"CustGrCode,omitempty"`
	CustGrId        string          `json:"CustGrId,omitempty"`
	ShowBalance     *bool           `json:"ShowBalance,omitempty"`
	ApixEInv        string          `json:"ApixEInv,omitempty"`
	Dimensions      []DimensionRef  `json:"Dimensions,omitempty"`
}

// CreateCustomerResponse represents the response from creating a customer.
type CreateCustomerResponse struct {
	ID   string `json:"Id"`
	Name string `json:"Name"`
}

// UpdateCustomerRequest represents the request body for updating a customer.
type UpdateCustomerRequest struct {
	ID               string          `json:"Id"`
	Name             string          `json:"Name,omitempty"`
	CountryCode      string          `json:"CountryCode,omitempty"`
	Address          string          `json:"Address,omitempty"`
	City             string          `json:"City,omitempty"`
	PostalCode       string          `json:"PostalCode,omitempty"`
	PhoneNo          string          `json:"PhoneNo,omitempty"`
	PhoneNo2         string          `json:"PhoneNo2,omitempty"`
	Email            string          `json:"Email,omitempty"`
	RegNo            string          `json:"RegNo,omitempty"`
	VatRegNo         string          `json:"VatRegNo,omitempty"`
	SalesInvLang     string          `json:"SalesInvLang,omitempty"`
	RefNoBase        string          `json:"RefNoBase,omitempty"`
	EInvPaymId       string          `json:"EInvPaymId,omitempty"`
	EInvOperator     *int            `json:"EInvOperator,omitempty"`
	BankAccount      string          `json:"BankAccount,omitempty"`
	CustGrCode       string          `json:"CustGrCode,omitempty"`
	CustGrId         string          `json:"CustGrId,omitempty"`
	Contact          string          `json:"Contact,omitempty"`
	ApixEInv         string          `json:"ApixEinv,omitempty"`
	GroupInv         *bool           `json:"GroupInv,omitempty"`
	PaymentDeadLine  *int            `json:"PaymentDeadLine,omitempty"`
	OverdueCharge    decimal.Decimal `json:"OverdueCharge,omitempty"`
	NotTDCustomer    *bool           `json:"NotTDCustomer,omitempty"`
}

// --- Vendors ---

// ListVendorsParams specifies parameters for listing vendors.
type ListVendorsParams struct {
	ID           string `json:"Id,omitempty"`
	RegNo        string `json:"RegNo,omitempty"`
	VatRegNo     string `json:"VatRegNo,omitempty"`
	Name         string `json:"Name,omitempty"`
	WithComments bool   `json:"WithComments,omitempty"`
	CommentsFrom string `json:"CommentsFrom,omitempty"`
	ChangedDate  string `json:"ChangedDate,omitempty"`
}

// VendorListItem represents a vendor in list results.
type VendorListItem struct {
	VendorID        string          `json:"VendorId"`
	VendorType      int             `json:"VendorType"`
	Name            string          `json:"Name"`
	RegNo           string          `json:"RegNo"`
	Contact         string          `json:"Contact"`
	PhoneNo         string          `json:"PhoneNo"`
	PhoneNo2        string          `json:"PhoneNo2"`
	Address         string          `json:"Address"`
	City            string          `json:"City"`
	County          string          `json:"County"`
	PostalCode      string          `json:"PostalCode"`
	CountryName     string          `json:"CountryName"`
	CountryCode     string          `json:"CountryCode"`
	FaxNo           string          `json:"FaxNo"`
	Email           string          `json:"Email"`
	HomePage        string          `json:"HomePage"`
	CurrencyCode    string          `json:"CurrencyCode"`
	PaymentDeadLine int             `json:"PaymentDeadLine"`
	BankAccount     string          `json:"BankAccount"`
	ReferenceNo     string          `json:"ReferenceNo"`
	OverdueCharge   decimal.Decimal `json:"OverdueCharge"`
	VatRegNo        string          `json:"VatRegNo"`
	VatAccountable  bool            `json:"VatAccountable"`
	VendorGroupID   string          `json:"VendorGroupId"`
	VendorGroupName string          `json:"VendorGroupName"`
	Dimensions      []DimensionRef  `json:"Dimensions"`
	ChangedDate     string          `json:"ChangedDate"`
}

// CreateVendorRequest represents the request body for creating a vendor.
type CreateVendorRequest struct {
	ID              string          `json:"Id,omitempty"`
	Name            string          `json:"Name"`
	RegNo           string          `json:"RegNo,omitempty"`
	VatAccountable  bool            `json:"VatAccountable"`
	VatRegNo        string          `json:"VatRegNo,omitempty"`
	CurrencyCode    string          `json:"CurrencyCode,omitempty"`
	PaymentDeadLine *int            `json:"PaymentDeadLine,omitempty"`
	OverDueCharge   decimal.Decimal `json:"OverDueCharge,omitempty"`
	RefNoBase       string          `json:"RefNoBase,omitempty"`
	Address         string          `json:"Address,omitempty"`
	CountryCode     string          `json:"CountryCode,omitempty"`
	County          string          `json:"County,omitempty"`
	City            string          `json:"City,omitempty"`
	PostalCode      string          `json:"PostalCode,omitempty"`
	PhoneNo         string          `json:"PhoneNo,omitempty"`
	PhoneNo2        string          `json:"PhoneNo2,omitempty"`
	HomePage        string          `json:"HomePage,omitempty"`
	Email           string          `json:"Email,omitempty"`
	VendorType      *int            `json:"VendorType,omitempty"`
	VendGrCode      string          `json:"VendGrCode,omitempty"`
	VendGrId        string          `json:"VendGrId,omitempty"`
	ReceiverName    string          `json:"ReceiverName,omitempty"`
	BankAccount     string          `json:"BankAccount,omitempty"`
	SWIFTBIC        string          `json:"SWIFT_BIC,omitempty"`
	Dimensions      []DimensionRef  `json:"Dimensions,omitempty"`
}

// CreateVendorResponse represents the response from creating a vendor.
type CreateVendorResponse struct {
	ID   string `json:"Id"`
	Name string `json:"Name"`
}

// UpdateVendorRequest represents the request body for updating a vendor.
type UpdateVendorRequest struct {
	ID              string          `json:"Id"`
	Name            string          `json:"Name,omitempty"`
	CountryCode     string          `json:"CountryCode,omitempty"`
	Address         string          `json:"Address,omitempty"`
	City            string          `json:"City,omitempty"`
	PostalCode      string          `json:"PostalCode,omitempty"`
	PhoneNo         string          `json:"PhoneNo,omitempty"`
	PhoneNo2        string          `json:"PhoneNo2,omitempty"`
	Email           string          `json:"Email,omitempty"`
	RegNo           string          `json:"RegNo,omitempty"`
	VatRegNo        string          `json:"VatRegNo,omitempty"`
	VatAccountable  *bool           `json:"VatAccountable,omitempty"`
	BankAccount     string          `json:"BankAccount,omitempty"`
	ReferenceNo     string          `json:"ReferenceNo,omitempty"`
	VendGrCode      string          `json:"VendGrCode,omitempty"`
	VendGrId        string          `json:"VendGrId,omitempty"`
	Dimensions      []DimensionRef  `json:"Dimensions,omitempty"`
	PaymentDeadLine *int            `json:"PaymentDeadLine,omitempty"`
	OverdueCharge   decimal.Decimal `json:"OverdueCharge,omitempty"`
}

// --- Items ---

// ListItemsParams specifies parameters for listing items.
type ListItemsParams struct {
	ID           string `json:"Id,omitempty"`
	Code         string `json:"Code,omitempty"`
	Description  string `json:"Description,omitempty"`
	LocationCode string `json:"LocationCode,omitempty"`
	Usage        *int   `json:"Usage,omitempty"`
	Type         *int   `json:"Type,omitempty"`
}

// ItemListItem represents an item/product in list results.
type ItemListItem struct {
	ItemID               string          `json:"ItemId"`
	Code                 string          `json:"Code"`
	Name                 string          `json:"Name"`
	NameEN               string          `json:"NameEN"`
	NameFI               string          `json:"NameFI"`
	NameRU               string          `json:"NameRU"`
	UnitOfMeasureName    string          `json:"UnitofMeasureName"`
	Type                 int             `json:"Type"`
	SalesPrice           decimal.Decimal `json:"SalesPrice"`
	InventoryQty         decimal.Decimal `json:"InventoryQty"`
	ReservedQty          decimal.Decimal `json:"ReservedQty"`
	VatTaxName           string          `json:"VatTaxName"`
	Usage                int             `json:"Usage"`
	SalesAccountCode     string          `json:"SalesAccountCode"`
	PurchaseAccountCode  string          `json:"PurchaseAccountCode"`
	InventoryAccountCode string          `json:"InventoryAccountCode"`
	ItemCostAccountCode  string          `json:"ItemCostAccountCode"`
	DiscountPct          decimal.Decimal `json:"DiscountPct"`
	LastPurchasePrice    decimal.Decimal `json:"LastPurchasePrice"`
	ItemUnitCost         decimal.Decimal `json:"ItemUnitCost"`
	InventoryCost        decimal.Decimal `json:"InventoryCost"`
	ItemGroupName        string          `json:"ItemGroupName"`
	DefLocName           string          `json:"DefLoc_Name"`
	EANCode              string          `json:"EANCode"`
	GTUCodes             string          `json:"GTUCodes"`
}

// CreateItemRequest represents a single item in a create items request.
type CreateItemRequest struct {
	Type             int    `json:"Type"`
	Usage            int    `json:"Usage"`
	Code             string `json:"Code"`
	Description      string `json:"Description"`
	EANCode          string `json:"EANCode,omitempty"`
	UOMName          string `json:"UOMName,omitempty"`
	DefLocationCode  string `json:"DefLocationCode,omitempty"`
	GTUCode          *int   `json:"GTUCode,omitempty"`
	DescriptionEN    string `json:"DescriptionEN,omitempty"`
	DescriptionRU    string `json:"DescriptionRU,omitempty"`
	DescriptionFI    string `json:"DescriptionFI,omitempty"`
	TaxID            string `json:"TaxId,omitempty"`
	ItemGrCode       string `json:"ItemGrCode,omitempty"`
	SalesAccCode     string `json:"SalesAccCode,omitempty"`
	PurchaseAccCode  string `json:"PurchaseAccCode,omitempty"`
	InventoryAccCode string `json:"InventoryAccCode,omitempty"`
	CostAccCode      string `json:"CostAccCode,omitempty"`
}

// CreateItemsWrapper wraps items for the send items endpoint.
type CreateItemsWrapper struct {
	Items []CreateItemRequest `json:"Items"`
}

// CreateItemResponse represents the response from creating an item.
type CreateItemResponse struct {
	ItemID string `json:"ItemId"`
	Code   string `json:"Code"`
}

// UpdateItemRequest represents the request body for updating an item.
type UpdateItemRequest struct {
	ID                   string          `json:"Id"`
	Code                 string          `json:"Code,omitempty"`
	Description          string          `json:"Description,omitempty"`
	SalesPrice           decimal.Decimal `json:"SalesPrice,omitempty"`
	ItemGrCode           string          `json:"ItemGrCode,omitempty"`
	DiscountPct          decimal.Decimal `json:"DiscountPct,omitempty"`
	EANCode              string          `json:"EANCode,omitempty"`
	NameEN               string          `json:"NameEN,omitempty"`
	LastPurchasePrice    decimal.Decimal `json:"LastPurchasePrice,omitempty"`
	SalesAccountCode     string          `json:"SalesAccountCode,omitempty"`
	InventoryAccountCode string          `json:"InventoryAccountCode,omitempty"`
	ItemCostAccountCode  string          `json:"ItemCostAccountCode,omitempty"`
	TaxID                string          `json:"TaxId,omitempty"`
	GTUCode              *int            `json:"GTUCode,omitempty"`
}

// --- Payments ---

// ListPaymentsParams specifies parameters for listing payments.
type ListPaymentsParams struct {
	PeriodStart time.Time `json:"PeriodStart"`
	PeriodEnd   time.Time `json:"PeriodEnd"`
	PaymentType *int      `json:"PaymentType,omitempty"`
	BankID      string    `json:"BankId,omitempty"`
	DateType    *int      `json:"DateType,omitempty"` // 0=document date, 1=changed date
}

// PaymentListItem represents a payment in list results.
type PaymentListItem struct {
	PIHId           string          `json:"PIHId"`
	BankName        string          `json:"BankName"`
	CounterPartType int             `json:"CounterPartType"`
	CounterPartName string          `json:"CounterPartName"`
	CurrencyCode    string          `json:"CurrencyCode"`
	CurrencyRate    decimal.Decimal `json:"CurrencyRate"`
	DocumentDate    string          `json:"DocumentDate"`
	DocumentNo      string          `json:"DocumentNo"`
	Direction       int             `json:"Direction"`
	Amount          decimal.Decimal `json:"Amount"`
	CounterPartID   string          `json:"CounterPartId"`
	EInvSentDate    string          `json:"EInvSentDate"`
	DocID           string          `json:"DocId"`
	ChangedDate     string          `json:"ChangedDate"`
	PaymAPIDetails  []PaymAPIDetail `json:"PaymAPIDetails"`
}

// PaymAPIDetail contains per-document payment details.
type PaymAPIDetail struct {
	PaymID       string          `json:"PaymId"`
	DocNo        string          `json:"DocNo"`
	DocAmount    decimal.Decimal `json:"DocAmount"`
	PaidAmount   decimal.Decimal `json:"PaidAmount"`
	CurrencyCode string          `json:"CurrencyCode"`
	CurrencyRate decimal.Decimal `json:"CurrencyRate"`
	DocID        string          `json:"DocId"`
}

// CreatePaymentRequest represents the request for creating a sales invoice payment.
type CreatePaymentRequest struct {
	BankID       string          `json:"BankId,omitempty"`
	IBAN         string          `json:"IBAN,omitempty"`
	CustomerName string          `json:"CustomerName"`
	InvoiceNo    string          `json:"InvoiceNo"`
	PaymentDate  string          `json:"PaymentDate"`
	RefNo        string          `json:"RefNo,omitempty"`
	Amount       decimal.Decimal `json:"Amount"`
	CurrencyCode string          `json:"CurrencyCode,omitempty"`
	CurrencyRate decimal.Decimal `json:"CurrencyRate,omitempty"`
}

// CreatePurchasePaymentRequest represents the request for creating a purchase invoice payment.
type CreatePurchasePaymentRequest struct {
	BankID       string          `json:"BankId,omitempty"`
	IBAN         string          `json:"IBAN,omitempty"`
	VendorName   string          `json:"VendorName"`
	PaymentDate  string          `json:"PaymentDate"`
	BillNo       string          `json:"BillNo"`
	RefNo        string          `json:"RefNo,omitempty"`
	Amount       decimal.Decimal `json:"Amount"`
	CurrencyCode string          `json:"CurrencyCode,omitempty"`
	CurrencyRate decimal.Decimal `json:"CurrencyRate,omitempty"`
}

// DeletePaymentParams specifies parameters for deleting a payment.
type DeletePaymentParams struct {
	ID string `json:"Id"`
}

// --- Taxes ---

// TaxItem represents a tax entry from the taxes list.
type TaxItem struct {
	TaxID  string          `json:"Id"`
	Code   string          `json:"Code"`
	Name   string          `json:"Name"`
	NameEN string          `json:"NameEN"`
	NameRU string          `json:"NameRU"`
	TaxPct decimal.Decimal `json:"TaxPct"`
}

// --- Accounts ---

// AccountItem represents a GL account from the chart of accounts.
type AccountItem struct {
	AccountID        string `json:"AccountID"`
	NonActive        string `json:"NonActive"`
	Code             string `json:"Code"`
	Name             string `json:"Name"`
	NameEN           string `json:"NameEN"`
	NameRU           string `json:"NameRU"`
	TaxName          string `json:"TaxName"`
	LinkedVendorName string `json:"LinkedVendorName"`
	IsParent         string `json:"IsParent"`
}

// ProjectItem represents a project.
type ProjectItem struct {
	Code    string `json:"Code"`
	Name    string `json:"Name"`
	EndDate string `json:"EndDate"`
}

// CostCenterItem represents a cost center.
type CostCenterItem struct {
	Code    string `json:"Code"`
	Name    string `json:"Name"`
	EndDate string `json:"EndDate"`
}

// Department represents a department.
type Department struct {
	Code      string `json:"Code"`
	Name      string `json:"Name"`
	NonActive bool   `json:"NonActive"`
}

// --- Reports ---

// CustomerDebtsParams specifies parameters for the customer debts report.
type CustomerDebtsParams struct {
	CustName    string `json:"CustName,omitempty"`    // Empty string = all customers
	CustID      string `json:"CustId,omitempty"`
	OverDueDays *int   `json:"OverDueDays,omitempty"` // Filter debts exceeding N days
	DebtDate    string `json:"DebtDate,omitempty"`    // YYYYMMdd; defaults to current date
}

// CustomerDebtItem represents a customer debt entry.
type CustomerDebtItem struct {
	PartnerName  string          `json:"PartnerName"`
	PartnerID    string          `json:"PartnerId"`
	DocType      string          `json:"DocType"` // SO=offer, MA=invoice, SBx=initial balance
	DocDate      string          `json:"DocDate"`
	DocNo        string          `json:"DocNo"`
	RefNo        string          `json:"RefNo"`
	DueDate      string          `json:"DueDate"`
	TotalAmount  decimal.Decimal `json:"TotalAmount"`
	PaidAmount   decimal.Decimal `json:"PaidAmount"`
	UnPaidAmount decimal.Decimal `json:"UnPaidAmount"`
	CurrencyCode string          `json:"CurrencyCode"`
	CurrencyRate decimal.Decimal `json:"CurrencyRate"`
}

// ProfitLossParams specifies parameters for the income statement report.
type ProfitLossParams struct {
	EndDate   string `json:"EndDate"`           // YYYYMMdd
	PerCount  int    `json:"PerCount"`           // Number of periods (months)
	DepFilter string `json:"DepFilter,omitempty"` // Department filter
}

// BalanceSheetParams specifies parameters for the balance sheet report.
type BalanceSheetParams struct {
	EndDate  string `json:"EndDate"`  // YYYYMMdd
	PerCount int    `json:"PerCount"` // Number of periods (months)
}

// FinancialReport represents the response from profit/loss and balance sheet endpoints.
type FinancialReport struct {
	ErrorMsg string              `json:"ErrorMsg"`
	Data     []FinancialReportRow `json:"Data"`
}

// FinancialReportRow represents a row in a financial report.
type FinancialReportRow struct {
	RDid        int                     `json:"RDid"`
	Description string                  `json:"Description"`
	RowType     int                     `json:"RowType"` // 1=description, 2=balance, 3=turnover, 4=formula
	Balance     []decimal.Decimal       `json:"Balance"`
	Details     []FinancialReportDetail `json:"Details"`
}

// FinancialReportDetail represents account-level detail in a financial report.
type FinancialReportDetail struct {
	AccountID   string            `json:"AccountId"`
	AccountCode string            `json:"AccountCode"`
	AccountName string            `json:"AccountName"`
	TypeID      int               `json:"TypeId"` // 1=assets, 2=liabilities, 3=revenue, 4=expenses
	Balance     []decimal.Decimal `json:"Balance"`
}
