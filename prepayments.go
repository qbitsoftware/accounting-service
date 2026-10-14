package accounting

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// PrepaymentProvider is an optional capability for accounting backends that
// model customer prepayments / advances (Estonian: ettemaks) as first-class
// documents. It is intentionally NOT part of the core Provider interface:
// callers type-assert for it (`pp, ok := provider.(PrepaymentProvider)`), so
// providers that lack a prepayment register need not implement it.
//
// Excellent Books, Merit and SmartAccounts implement this. Each models a
// prepayment differently:
//   - EB: an unallocated receipt row carrying a caller-assigned CUPNr; apply and
//     unallocate are two-row net-zero receipts.
//   - Merit: a customer advance via PrePayments/ForCustomer; Apply/Unallocate are
//     unsupported (Merit's API cannot allocate — callers net via a credit bank).
//   - SmartAccounts: a payment carrying a `prepayment` object; Apply is a
//     net-zero payment pairing a CLIENT_INVOICE row with a PREPAYMENT_PAYMENT row
//     referencing the parked payment's id. Unallocate is unsupported (SA accepts
//     credit notes on paid invoices directly, so it is unnecessary).
//
// The PrepaymentNo (EB CUPNr) is caller-assigned and must be unique in the
// provider. klubio derives it from its own already-unique document numbers.
// SmartAccounts instead assigns its own id (returned as Prepayment.Number/DocID),
// which the caller stores and passes back as PrepaymentNo when applying.
type PrepaymentProvider interface {
	// CreatePrepayment records an unallocated customer advance — money the
	// customer has with us that is not yet tied to an invoice.
	CreatePrepayment(ctx context.Context, input CreatePrepaymentInput) (*Prepayment, error)

	// ApplyPrepayment settles (part of) an invoice from an existing prepayment,
	// moving no cash. In EB this is a two-row receipt: the invoice row is
	// positive, the prepayment row is negative.
	ApplyPrepayment(ctx context.Context, input ApplyPrepaymentInput) error

	// UnallocateToPrepayment frees an already-paid amount off an invoice and
	// parks it as a new prepayment, re-opening the invoice by that amount. This
	// is the prerequisite for crediting a fully-paid invoice: EB refuses a
	// credit note larger than the invoice's open amount, so the paid portion
	// must first become a prepayment. In EB this is a two-row receipt: the
	// invoice row is negative, the new prepayment row is positive.
	UnallocateToPrepayment(ctx context.Context, input UnallocateToPrepaymentInput) (*Prepayment, error)

	// ListPrepayments returns the customer's prepayments with their unapplied
	// remainder, derived from the provider's receipt history within the window.
	ListPrepayments(ctx context.Context, input ListPrepaymentsInput) ([]Prepayment, error)
}

// Prepayment is a customer advance / on-account credit (ettemaks).
type Prepayment struct {
	// Number is the provider prepayment number (EB CUPNr; Merit DocNo).
	Number string
	// DocID is the provider's internal document GUID, when it exposes a stable
	// one (Merit getcustdebtrep DocId). EB leaves this empty — there Number
	// (CUPNr) is the stable key. Prefer DocID for identity when present, since
	// Merit DocNo is caller-assigned and need not be unique.
	DocID        string
	CustomerCode string
	// CustomerName is the provider's display name for the customer (Merit
	// PartnerName). Empty when the provider's prepayment source carries no name
	// (e.g. EB receipt rows expose only the customer code).
	CustomerName string
	// Amount is the original advance; Remaining is the still-unapplied part.
	Amount    decimal.Decimal
	Remaining decimal.Decimal
	Currency  string
	Date      time.Time
	Comment   string
}

type CreatePrepaymentInput struct {
	CustomerCode string
	// PrepaymentNo is the CUPNr the caller assigns; must be unique in the provider.
	// SmartAccounts ignores this on input (it assigns its own paymentId, returned
	// as Prepayment.Number/DocID); pass it in the document field for traceability.
	PrepaymentNo string
	Amount       decimal.Decimal
	Currency     string
	PaymentDate  time.Time
	// BankID is the provider payment-method code (EB PayMode); required by EB. For
	// SmartAccounts it is the bank-account NAME the advance receipt books into.
	BankID string
	// Account is the GL prepayment (customer-advance liability) account code, for
	// providers that post the advance to a ledger account directly (SmartAccounts
	// prepayment.account, e.g. "21211"). EB/Merit ignore it (they use PayMode/bank).
	Account string
	Comment string
	// AccountKind is the kind of account BankID names (SmartAccounts only):
	// PaymentAccountCash parks the advance in a cash desk (accountType=CASH).
	// The default sends exactly what was sent before the field existed.
	AccountKind PaymentAccountKind
}

type ApplyPrepaymentInput struct {
	CustomerCode string
	InvoiceNo    string
	PrepaymentNo string
	Amount       decimal.Decimal
	Currency     string
	PaymentDate  time.Time
	BankID       string
	// Document is written to the application's document field so the caller
	// can later recognise the application as its own: SmartAccounts (both
	// ApplyPrepayment and ApplyPrepaymentReceipt) and Excellent Books'
	// ApplyPrepaymentReceipt (header Comment; its ApplyPrepayment ignores it).
	// Empty ⇒ not sent, which is the request every caller sent before v0.15.0.
	Document string
}

type UnallocateToPrepaymentInput struct {
	CustomerCode string
	// InvoiceNo is the invoice to free money from (its provider SerNr).
	InvoiceNo string
	// PrepaymentNo is the new CUPNr that will hold the freed money.
	PrepaymentNo string
	Amount       decimal.Decimal
	Currency     string
	PaymentDate  time.Time
	BankID       string
	Comment      string
}

type ListPrepaymentsInput struct {
	CustomerCode string
	Since        time.Time
	Until        time.Time
}
