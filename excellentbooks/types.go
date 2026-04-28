package excellentbooks

import "encoding/json"

// Response is the top-level JSON response wrapper from Excellent Books.
// The data object contains a register-named array of records.
type Response struct {
	Data     json.RawMessage `json:"data"`
	Messages []string        `json:"messages,omitempty"`
}

// ResponseMeta holds metadata from the data envelope.
type ResponseMeta struct {
	Register      string `json:"@register"`
	Sequence      string `json:"@sequence"`
	SystemVersion string `json:"@systemversion"`
}

// Invoice represents a sales invoice (register IVVc).
type Invoice struct {
	SerNr          string       `json:"SerNr"`
	InvDate        string       `json:"InvDate"`
	CustCode       string       `json:"CustCode"`
	PayDate        string       `json:"PayDate"`
	Addr0          string       `json:"Addr0"`
	Addr1          string       `json:"Addr1"`
	Addr2          string       `json:"Addr2"`
	Addr3          string       `json:"Addr3"`
	ClientContact  string       `json:"ClientContact"`
	PayDeal        string       `json:"PayDeal"`
	OKFlag         string       `json:"OKFlag"`
	InvType        string       `json:"InvType"`
	Objects        string       `json:"Objects"`
	ARAcc          string       `json:"ARAcc"`
	InvComment     string       `json:"InvComment"`
	SalesMan       string       `json:"SalesMan"`
	TransDate      string       `json:"TransDate"`
	CurncyCode     string       `json:"CurncyCode"`
	Sum0           string       `json:"Sum0"` // Discount amount
	Sum1           string       `json:"Sum1"` // Net amount (excl VAT)
	Sum3           string       `json:"Sum3"` // VAT amount
	Sum4           string       `json:"Sum4"` // Total (incl VAT)
	VATNr          string       `json:"VATNr"`
	CalcFinRef     string       `json:"CalcFinRef"`
	RefStr         string       `json:"RefStr"`
	CredInv        string       `json:"CredInv"` // Credit invoice reference
	OrderNr        string       `json:"OrderNr"`
	Location       string       `json:"Location"`
	RegNr1         string       `json:"RegNr1"`
	InvCountry     string       `json:"InvCountry"`
	Rows           []InvoiceRow `json:"rows,omitempty"`
	OurContact     string       `json:"OurContact"`
	ExportFlag     string       `json:"ExportFlag"`
	Sequence       string       `json:"@sequence"`
	URL            string       `json:"@url"`
	BaseSum4       string       `json:"BaseSum4"`
	BKEInvSentDate string       `json:"BKEInvSentDate"`
}

// InvoiceRow represents a line item on an invoice.
type InvoiceRow struct {
	RowNumber string `json:"@rownumber"`
	Stp       string `json:"stp"`       // Row type: 1=normal, 3=credit
	ArtCode   string `json:"ArtCode"`   // Article/item code
	Quant     string `json:"Quant"`     // Quantity
	Price     string `json:"Price"`     // Unit price
	Sum       string `json:"Sum"`       // Line total (excl VAT)
	VRebate   string `json:"vRebate"`   // Discount %
	SalesAcc  string `json:"SalesAcc"`  // Sales account
	Spec      string `json:"Spec"`      // Description
	VATCode   string `json:"VATCode"`   // VAT code
	UnitCode  string `json:"UnitCode"`  // Unit
	Objects   string `json:"Objects"`   // Cost center / dimension
	OrdRow    string `json:"OrdRow"`    // Original order row reference
}

// Customer represents a contact (register CUVc).
type Customer struct {
	Code         string `json:"Code"`
	Name         string `json:"Name"`
	Person       string `json:"Person"`
	Phone        string `json:"Phone"`
	Fax          string `json:"Fax"`
	Email        string `json:"eMail"`
	VATNr        string `json:"VATNr"`
	CountryCode  string `json:"CountryCode"`
	CurncyCode   string `json:"CurncyCode"`
	PayDeal      string `json:"PayDeal"`
	RegNr1       string `json:"RegNr1"`
	RegNr2       string `json:"RegNr2"`
	InvAddr0     string `json:"InvAddr0"`
	InvAddr1     string `json:"InvAddr1"`
	InvAddr2     string `json:"InvAddr2"`
	InvAddr3     string `json:"InvAddr3"`
	InvAddr4     string `json:"InvAddr4"`
	DelAddr0     string `json:"DelAddr0"`
	DelAddr1     string `json:"DelAddr1"`
	DelAddr2     string `json:"DelAddr2"`
	WWWAddr      string `json:"wwwAddr"`
	CustType     string `json:"CustType"`  // 0=customer, 1=vendor, 2=both
	DateCreated  string `json:"DateCreated"`
	DateChanged  string `json:"DateChanged"`
	BlockedFlag  string `json:"blockedFlag"`
	SalesMan     string `json:"SalesMan"`
	Comment      string `json:"Comment"`
	Objects      string `json:"Objects"`
	BankAccount  string `json:"BankAccount"`
	Sequence     string `json:"@sequence"`
	URL          string `json:"@url"`
}

// Item represents an article/product (register INVc).
type Item struct {
	Code       string `json:"Code"`
	Name       string `json:"Name"`
	Unittext   string `json:"Unittext"`
	UPrice1    string `json:"UPrice1"`   // Sales price
	InPrice    string `json:"InPrice"`   // Purchase price
	ItemType   string `json:"ItemType"`  // 1=stock, 2=service
	Group      string `json:"Group"`
	SalesAcc   string `json:"SalesAcc"`
	VATCode    string `json:"VATCode"`
	Terminated string `json:"Terminated"` // 0=active, 1=terminated
	BarCode    string `json:"BarCode"`
	Sequence   string `json:"@sequence"`
	URL        string `json:"@url"`
}

// Receipt represents an incoming payment/receipt (register IPVc).
type Receipt struct {
	SerNr      string       `json:"SerNr"`
	TransDate  string       `json:"TransDate"`
	PayMode    string       `json:"PayMode"`
	Comment    string       `json:"Comment"`
	OKFlag     string       `json:"OKFlag"`
	PayCurCode string       `json:"PayCurCode"`
	Objects    string       `json:"Objects"`
	Rows       []ReceiptRow `json:"rows,omitempty"`
	Sequence   string       `json:"@sequence"`
	URL        string       `json:"@url"`
}

// ReceiptRow represents a line item on a receipt.
type ReceiptRow struct {
	RowNumber string `json:"@rownumber"`
	Stp       string `json:"stp"`
	InvoiceNr string `json:"InvoiceNr"`
	CustCode  string `json:"CustCode"`
	CustName  string `json:"CustName"`
	RecVal    string `json:"RecVal"`
	PayDate   string `json:"PayDate"`
	BankVal   string `json:"BankVal"`
	RecCurncy string `json:"RecCurncy"`
	Objects   string `json:"Objects"`
	Comment   string `json:"Comment"`
}

// VATCode represents a VAT/tax code (register VATCodeBlock).
// Standard Books returns these wrapped: { VATCodeBlock: { rows: [VATCode...] } }.
type VATCode struct {
	Code        string `json:"VATCode"`     // VAT code (e.g. "22", "24", "EU24")
	Comment     string `json:"Comment"`     // Description (e.g. "Käibemaksuga 22%")
	ExVatpr     string `json:"ExVatpr"`     // Exclusive VAT % (e.g. "22.00")
	IncVatpr    string `json:"IncVatpr"`    // Inclusive VAT %
	SalesVATAcc string `json:"SalesVATAcc"` // GL account for output VAT
	PurchVATAcc string `json:"PurchVATAcc"` // GL account for input VAT
	ValidUntil  string `json:"ValidUntil"`  // Empty = active; "YYYY-MM-DD" = expired after that date
	ValidFrom   string `json:"ValidFrom"`
}

// GLAccount represents a chart-of-accounts entry (register AccVc).
// Standard Books returns the account number under "AccNumber" and the active
// flag under lowercase "blockedFlag" (0 = active, 1 = blocked).
type GLAccount struct {
	Code        string `json:"AccNumber"`
	Comment     string `json:"Comment"`
	AccType     string `json:"AccType"`
	BlockedFlag string `json:"blockedFlag"`
	Sequence    string `json:"@sequence"`
	URL         string `json:"@url"`
}

// Object represents a cost center / dimension entry (register ObjVc).
type Object struct {
	Code     string `json:"Code"`
	Comment  string `json:"Comment"`
	OTCode   string `json:"OTCode"` // Object type code
	Closed   string `json:"Closed"`
	Sequence string `json:"@sequence"`
	URL      string `json:"@url"`
}

// Project represents a project entry (register PRVc).
type Project struct {
	Code     string `json:"Code"`
	Comment  string `json:"Comment"`
	Closed   string `json:"Closed"`
	Sequence string `json:"@sequence"`
	URL      string `json:"@url"`
}

// Department represents a department entry (register DepVc).
type Department struct {
	Code     string `json:"Code"`
	Comment  string `json:"Comment"`
	Closed   string `json:"Closed"`
	Sequence string `json:"@sequence"`
	URL      string `json:"@url"`
}

// PurchaseInvoice represents a purchase invoice (register VIVc).
type PurchaseInvoice struct {
	SerNr      string `json:"SerNr"`
	InvDate    string `json:"InvDate"`
	VECode     string `json:"VECode"`
	VEName     string `json:"VEName"`
	DueDate    string `json:"DueDate"`
	PayVal     string `json:"PayVal"`  // Total incl VAT
	VATVal     string `json:"VATVal"`  // VAT amount
	CurncyCode string `json:"CurncyCode"`
	OKFlag     string `json:"OKFlag"`
	InvoiceNr  string `json:"InvoiceNr"`
	RefStr     string `json:"RefStr"`
	Comment    string `json:"Comment"`
	TransDate  string `json:"TransDate"`
	Sequence   string `json:"@sequence"`
	URL        string `json:"@url"`
}
