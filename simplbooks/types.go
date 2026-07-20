package simplbooks

import (
	"strings"

	"github.com/shopspring/decimal"
)

// Decimal is a decimal.Decimal that tolerates the range of JSON shapes
// SimplBooks uses for numeric fields: bare numbers (13.73), quoted numbers
// ("3.5"), null and empty string. Any of those that isn't a real number
// decodes to zero rather than failing the whole response.
type Decimal struct {
	decimal.Decimal
}

// UnmarshalJSON accepts numbers, quoted numbers, null and "".
func (d *Decimal) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" || s == `""` {
		d.Decimal = decimal.Zero
		return nil
	}
	return d.Decimal.UnmarshalJSON(b)
}

// Invoice is a sales invoice. The full field set is returned by invoices/get;
// invoices/list returns a subset (no reference / address / rows).
type Invoice struct {
	ID                      int     `json:"id"`
	ClientID                int     `json:"client_id"`
	ClientName              string  `json:"client_name"`
	ClientRegNo             string  `json:"client_reg_no"`
	ClientVatNo             string  `json:"client_vat_no"`
	ClientAddressStreet     string  `json:"client_address_street"`
	ClientAddressCity       string  `json:"client_address_city"`
	ClientAddressPostalCode string  `json:"client_address_postal_code"`
	ClientAddressCounty     string  `json:"client_address_county"`
	ClientAddressCountry    string  `json:"client_address_country"`
	Created                 string  `json:"created"`
	TransactionDate         string  `json:"transaction_date"`
	Due                     string  `json:"due"`
	Number                  string  `json:"number"`
	Reference               string  `json:"reference"`
	Sum                     Decimal `json:"sum"`
	Vat                     Decimal `json:"vat"`
	Rounding                Decimal `json:"rounding"`
	TotalSum                Decimal `json:"total_sum"`
	CurrencyName            string  `json:"currency_name"`
	CurrencyRate            Decimal `json:"currency_rate"`
	Sent                    string  `json:"sent"`
	Paid                    string  `json:"paid"`
	// Remnant is the unpaid balance. It is only returned by invoices/get —
	// invoices/list omits it entirely — so it is a pointer to tell "absent
	// from this projection" apart from a genuine zero (fully paid).
	Remnant *Decimal `json:"remnant"`
	AdditionalInfo          string  `json:"additional_info"`
	CreditInvoiceFor        int     `json:"credit_invoice_for"`
	Proforma                bool    `json:"proforma"`
	Language                string  `json:"language"`

	// Rows is populated from the sibling "Task" array on invoices/get.
	Rows []InvoiceRow `json:"-"`
}

// InvoiceRow is one line of an invoice (SimplBooks "Task").
type InvoiceRow struct {
	ID              int     `json:"id"`
	ArticleID       int     `json:"article_id"`
	Code            string  `json:"code"`
	Name            string  `json:"name"`
	Contents        string  `json:"contents"`
	Unit            string  `json:"unit"`
	Amount          Decimal `json:"amount"`
	PricePerUnit    Decimal `json:"price_per_unit"`
	Vat             Decimal `json:"vat"`
	VatTypeID       int     `json:"vat_type_id"`
	Discount        Decimal `json:"discount"`
	IncomeAccountID int     `json:"income_account_id"`
}

// ClientRecord is a customer/partner card (SimplBooks "Client"). Named
// ClientRecord rather than Client to avoid clashing with the API *Client type.
type ClientRecord struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	RegNo             string `json:"reg_no"`
	VatNo             string `json:"vat_no"`
	AddressStreet     string `json:"address_street"`
	AddressCity       string `json:"address_city"`
	AddressPostalCode string `json:"address_postal_code"`
	AddressCounty     string `json:"address_county"`
	AddressCountry    string `json:"address_country"`
	Webpage           string `json:"webpage"`
	Email             string `json:"e_mail"`
	Phone             string `json:"phone"`
	AccountNo         string `json:"account_no"`
	Bank              string `json:"bank"`
	SwiftCode         string `json:"swift_code"`
}

// Article is an inventory item or service.
type Article struct {
	ID              int     `json:"id"`
	Code            string  `json:"code"`
	Ean             string  `json:"ean"`
	Name            string  `json:"name"`
	Contents        string  `json:"contents"`
	Unit            string  `json:"unit"`
	Amount          Decimal `json:"amount"`
	PricePerUnit    Decimal `json:"price_per_unit"`
	IsInventory     bool    `json:"is_inventory"`
	Active          bool    `json:"active"`
	SalesVatTypeID  int     `json:"sales_vat_type_id"`
	IncomeAccountID int     `json:"income_account_id"`
}

// Incoming is a customer receipt (money in).
type Incoming struct {
	ID              int     `json:"id"`
	IncomeAccountID int     `json:"income_account_id"`
	IncomeSum       Decimal `json:"income_sum"`
	IncomeDate      string  `json:"income_date"`
	Description     string  `json:"description"`
	CurrencyName    string  `json:"currency_name"`
	ClientID        int     `json:"client_id"`
	// Bound reports whether the receipt has been fully allocated to invoices.
	// SimplBooks exposes no per-receipt remaining amount, so this boolean is the
	// only signal that an advance still has money left on it: false = something
	// remains, true = fully consumed. Verified live.
	Bound bool `json:"bound"`
}

// VatType is a VAT/tax class.
type VatType struct {
	ID         int     `json:"id"`
	Name       string  `json:"name"`
	VatPercent Decimal `json:"vat_percent"`
	IsSales    bool    `json:"is_sales"`
	IsPurchase bool    `json:"is_purchase"`
	Active     bool    `json:"active"`
}

// FinancialAccount is a chart-of-accounts (GL) entry.
type FinancialAccount struct {
	ID     int    `json:"id"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// IncomeAccount is a bank account or cash register (SimplBooks tags these under
// "Bank accounts and cash").
type IncomeAccount struct {
	ID                 int    `json:"id"`
	AccountName        string `json:"account_name"`
	BankName           string `json:"bank_name"`
	AccountNo          string `json:"account_no"`
	IbanNumber         string `json:"iban_number"`
	SwiftCode          string `json:"swift_code"`
	AccountDescription string `json:"account_description"`
	Active             bool   `json:"active"`
}
