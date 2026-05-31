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
// Only Excellent Books implements this today. EB represents a prepayment as an
// unallocated receipt row carrying a CUPNr (prepayment number); see the
// excellentbooks adapter for the exact wire format.
//
// The PrepaymentNo (CUPNr) is caller-assigned and must be unique in the
// provider. klubio derives it from its own already-unique document numbers
// (e.g. the credit-note number) — EB accepts alphanumeric values.
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
	// Number is the provider prepayment number (EB CUPNr).
	Number       string
	CustomerCode string
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
	PrepaymentNo string
	Amount       decimal.Decimal
	Currency     string
	PaymentDate  time.Time
	// BankID is the provider payment-method code (EB PayMode); required by EB.
	BankID  string
	Comment string
}

type ApplyPrepaymentInput struct {
	CustomerCode string
	InvoiceNo    string
	PrepaymentNo string
	Amount       decimal.Decimal
	Currency     string
	PaymentDate  time.Time
	BankID       string
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
