package accounting

import (
	"time"

	"github.com/shopspring/decimal"
)

type CreateInvoiceInput struct {
	CustomerID          string
	CustomerName        string
	CustomerRegNo       string
	CustomerVATNo       string
	CustomerEmail       string
	CustomerAddress     string
	CustomerCountryCode string
	CustomerCounty      string // Directo tax region / Asumaa for the inline customer (e.g. "Harjumaa")
	CustomerType        string // Directo customer_type: "0"=company, "1"=private, "2"=government
	// VATZone is Directo's KM maa (sql field `maa`): "0" local, "1" EU,
	// "2" export. It is a DECLARATION classifier, not a tax calculation —
	// which VAT return the document lands in (zone 1 feeds the VD / EC sales
	// list). The tax itself still comes from each line's TaxID. Empty = not
	// sent, and Directo falls back to the customer card's zone, then to local.
	// Directo-only; every other adapter ignores it.
	VATZone string
	DocDate time.Time
	DueDate time.Time
	// SentDate is the date the invoice was actually delivered to the customer by
	// the calling system. Providers that track a delivery state on the document
	// stamp it at creation, so an invoice that already went out from klubio does
	// not sit in the accounting UI marked "not sent". Zero = leave the provider's
	// own default (unsent). Only SimplBooks acts on this today; other adapters
	// ignore it.
	SentDate  time.Time
	InvoiceNo string
	// OfficialNo is the caller's own document number, written to the
	// provider's secondary number field (Excellent Books "Ametlik
	// seerianumber", OfficialSerNr) beside the provider's own SerNr. Empty =
	// not sent. Excellent Books only; other adapters ignore it.
	OfficialNo  string
	RefNo       string
	Currency    string
	Lines       []CreateInvoiceLineInput
	TotalAmount decimal.Decimal // Net subtotal, before tax (for validation)
	// TotalWithVAT is the gross document total the caller has already billed —
	// what the customer owes on the invoice they already hold. Zero = not
	// stated, and every reconciliation that depends on it is then skipped.
	//
	// It exists because the caller and the provider compute a document's VAT
	// from different bases, and neither is wrong: klubio rounds each LINE's VAT
	// to a cent and sums, SimplBooks rounds once over the DOCUMENT's net. Sum of
	// rounded is not rounded of sum, so the two can land a cent apart on
	// identical lines. Stating what was billed lets an adapter reconcile the
	// difference where the provider offers a way (SimplBooks: a rounding line),
	// and report it where it does not.
	TotalWithVAT  decimal.Decimal
	Comment       string
	FooterComment string
	AutoConfirm   bool // If true, sets OKFlag=1 on the invoice (confirms immediately)
	// PaymentTermCode references a code in the provider's payment-term register
	// (Directo maksetingimus). Forwarded as the invoice's paymentterm attribute.
	// When empty the provider falls back to the customer's default term — which
	// Directo flags as required ("T-tingimus puudu"), so set it.
	PaymentTermCode string
}

// LineDimension represents a dimension to attach to a Merit invoice row.
type LineDimension struct {
	DimID      int    // Dimension type ID
	DimValueID string // Dimension value GUID
	DimCode    string // Dimension value code
}

type CreateInvoiceLineInput struct {
	Code        string
	Description string
	Quantity    decimal.Decimal
	UnitPrice   decimal.Decimal
	// TaxRate is the line's VAT rate as a PERCENT (24 for 24%). TaxID names that
	// rate in the provider's own register and is what actually goes on the wire;
	// this is the number itself, so an adapter can work out what the provider
	// will compute without a round trip to resolve the code. Read only
	// alongside CreateInvoiceInput.TotalWithVAT.
	TaxRate        decimal.Decimal
	TaxID          string
	AccountCode    string
	Type           *int            // Item type: 1=product, 2=package, 3=service (default: 3)
	UOMName        string          // Unit of measure (e.g., "pcs", "hrs", "kg")
	ProjectCode    string          // Merit project code (dimension) — flat field for v1 compat
	CostCenterCode string          // Merit cost center code (dimension) — flat field for v1 compat
	Dimensions     []LineDimension // Merit v2 dimensions array
}

type CreateCustomerInput struct {
	Name string
	// Code is the customer identifier in the provider's namespace. Optional —
	// most providers (Merit) auto-assign their own ID. Required for Excellent
	// Books when the EB instance has no number series configured for the
	// Customer (CUVc) register; without it EB rejects with error 1557 ("Ei
	// ole määratud numbriseerias"). Caller (klubio) generates a stable code
	// from athlete profile ID / company registry code.
	Code        string
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
	RefNoBase   string // Base for per-customer reference number generation in Merit
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
	RefNoBase   *string // Base for per-customer reference number generation in Merit
}

type CreatePaymentInput struct {
	CustomerName string
	CustomerCode string // Customer code/ID — required by Excellent Books receipts; Merit uses CustomerName
	// PaymentNo is the receipt's own document number. REQUIRED for Directo:
	// the XML Direct receipt import rejects documents without a number
	// (result type 12, "Missing document identificator"). Callers should
	// derive it deterministically (e.g. from the invoice number) so retries
	// are idempotent.
	PaymentNo   string
	InvoiceNo   string
	PaymentDate time.Time
	Amount      decimal.Decimal
	Currency    string
	BankID      string
	// AutoConfirm books the receipt immediately (Directo: kinnitatud).
	// An unconfirmed receipt does not settle its invoice.
	AutoConfirm bool
}

type ListInvoicesInput struct {
	PeriodStart time.Time
	PeriodEnd   time.Time
	// CustomerCode restricts the result to invoices belonging to a specific
	// customer card. Currently honored by Excellent Books only — Merit/Directo
	// ignore it and return the full date-range result.
	CustomerCode string
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
	ID               string
	Code             *string
	Description      *string
	SalesPrice       *decimal.Decimal
	TaxID            *string
	SalesAccountCode *string
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
	CustomerCounty      string // Directo tax region / Asumaa for the inline customer
	// VATZone mirrors CreateInvoiceInput.VATZone. Send the same zone the
	// credited invoice carried, or the credit note lands in a different VAT
	// declaration than the document it reverses.
	VATZone string
	DocDate time.Time
	DueDate time.Time
	// SentDate mirrors CreateInvoiceInput.SentDate — the date the credit note was
	// delivered to the customer. Zero = leave the provider's default (unsent).
	SentDate  time.Time
	InvoiceNo string
	// OfficialNo is the caller's own document number, written to the
	// provider's secondary number field (Excellent Books "Ametlik
	// seerianumber", OfficialSerNr) beside the provider's own SerNr. Empty =
	// not sent. Excellent Books only; other adapters ignore it.
	OfficialNo  string
	RefNo       string
	Currency    string
	Lines       []CreateInvoiceLineInput
	TotalAmount decimal.Decimal // must be negative (net subtotal before tax)
	// TotalWithVAT mirrors CreateInvoiceInput's, and matters more here: a credit
	// that does not exactly undo its original leaves a permanent cent on the
	// customer's balance that nothing will ever clear. Verified against
	// SimplBooks — an original of 35.00 was credited -35.01 before this was
	// carried across. State it NEGATIVE, the sign the credit note itself is in.
	TotalWithVAT      decimal.Decimal
	Comment           string
	FooterComment     string
	OriginalInvoiceNo string
	// PaymentTermCode references a code in the provider's payment-term
	// register (Excellent Books PDVc; e.g. "K" for cash). When set, the
	// adapter forwards it as the invoice's PayDeal field. When empty, the
	// adapter omits the field and the provider falls back to the customer's
	// default payment term.
	PaymentTermCode string
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
