package accounting

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

// Receipt-level operations (v0.15.0).
//
// A caller that keeps "one writer per ledger" needs more from a provider than
// "post a payment": it has to know the id of the receipt it wrote, recognise its
// own receipts among the accountant's, see each receipt's rows separately, and
// learn which receipts were deleted. These are optional capabilities, type-
// asserted like PrepaymentProvider, so a provider that cannot answer them simply
// does not implement them — and a caller asking anyway gets
// ErrUnsupportedProvider, never a silent fallback to the old behaviour.
//
// SmartAccounts implements all of them except ReceiptReverser. Excellent Books
// implements ReceiptCreator, PrepaymentReceiptApplier, InvoiceReceiptLister,
// ReceiptGetter, PaymentChangeLister and ReceiptReverser
// (excellentbooks_receipts.go). Nothing here changes what any existing method
// sends.

// ReceiptCreator books a receipt and returns what the provider made of it.
type ReceiptCreator interface {
	CreateReceipt(ctx context.Context, input CreatePaymentInput) (*CreatedReceipt, error)
}

// PrepaymentReceiptApplier applies a prepayment like
// PrepaymentProvider.ApplyPrepayment and returns what the provider made of the
// application — its id above all. Opt-in: ApplyPrepayment is unchanged.
type PrepaymentReceiptApplier interface {
	ApplyPrepaymentReceipt(ctx context.Context, input ApplyPrepaymentInput) (*CreatedReceipt, error)
}

// InvoiceReceiptLister reads one customer's receipts.
type InvoiceReceiptLister interface {
	ListInvoiceReceipts(ctx context.Context, input ListInvoiceReceiptsInput) ([]Receipt, error)
}

// ReceiptGetter reads one receipt by id. A receipt that is gone answers an
// error for which IsNotFound is true — and only then may the caller conclude
// it is gone.
//
// For SmartAccounts that conclusion is not yet proven safe on its own: what
// payments:get answers for a deleted id, and whether an existing receipt over a
// year old always comes back by id, are open sandbox questions (design §7 item
// 7). Until both are recorded a caller must pair a not-found with a second
// signal — the id in the change feed's Deleted — before treating a receipt as
// deleted.
type ReceiptGetter interface {
	GetReceipt(ctx context.Context, id string) (*Receipt, error)
}

// PaymentChangeLister reads every receipt changed or deleted in a window.
type PaymentChangeLister interface {
	ListPaymentChanges(ctx context.Context, input PaymentChangesInput) (*PaymentChanges, error)
}

// CashAccountLister lists the provider's cash desks as Banks (ID = Name =
// the desk's name, AccountCode = its GL account). Kept apart from ListBanks on
// purpose: a cash desk is not a bank, and merging them into ListBanks would let
// a bank picker offer one.
type CashAccountLister interface {
	ListCashAccounts(ctx context.Context) ([]Bank, error)
}

// NettingBankLister names the bank accounts the provider itself marks as its
// default for netting credit invoices against invoices (SmartAccounts'
// bankaccounts.forNetting). Read for setup advice only.
type NettingBankLister interface {
	ListNettingBanks(ctx context.Context) ([]string, error)
}

// BudgetReporter reports how many requests this process has sent the provider
// in its budget window. A floor and a hint, never a decision.
type BudgetReporter interface {
	BudgetUsed() int
}

// CreatedReceipt is the receipt a provider booked.
type CreatedReceipt struct {
	// ID is the provider's receipt id. It MAY be empty: the receipt exists all
	// the same, so an empty id is never a failure — the caller recognises the
	// receipt later by its Document.
	ID         string
	Number     string
	Amount     decimal.Decimal
	InvoiceID  string
	CustomerID string
}

// AllocationKind classifies what one receipt row means for one invoice. The
// SDK does not classify — only the caller knows which documents are its own
// credit notes and advances — but the vocabulary is shared.
type AllocationKind string

const (
	// AllocMoney is money reaching the invoice.
	AllocMoney AllocationKind = "money"
	// AllocFromAdvance is an amount drawn from an advance (a PREPAYMENT_PAYMENT
	// row funds it), not new money.
	AllocFromAdvance AllocationKind = "from_advance"
	// AllocNetting is a credit note closed against its original.
	AllocNetting AllocationKind = "netting"
	// AllocUnknown is a row that cannot be classified safely: an unresolvable
	// document, an unreadable receipt, an advance draw shared by several invoices.
	AllocUnknown AllocationKind = "unknown"
)

// ReceiptRow is one allocation row of a receipt.
type ReceiptRow struct {
	// Type is the provider's row type (SmartAccounts CLIENT_INVOICE,
	// VENDOR_INVOICE, PREPAYMENT_PAYMENT).
	Type string
	// DocID is the document the row points at: the invoice, or for
	// PREPAYMENT_PAYMENT the advance's own receipt id.
	DocID       string
	Amount      decimal.Decimal
	Description string
}

// Receipt is one provider receipt with every row kept apart.
type Receipt struct {
	ID          string
	Number      string
	Document    string
	Comment     string
	OrderNumber string // cash-desk order number, when the provider assigns one
	Date        time.Time
	// PartnerType is the provider's partner type ("CLIENT" | "VENDOR").
	PartnerType string
	// AccountType is "BANK" or "CASH".
	AccountType string
	AccountName string
	Currency    string
	// Amount is the whole receipt.
	Amount       decimal.Decimal
	CustomerID   string
	CustomerName string
	Rows         []ReceiptRow
	// AdvanceAmount is the advance this receipt PARKS (SmartAccounts' top-level
	// prepayment object), zero when it parks none.
	AdvanceAmount decimal.Decimal
	// AdvanceSourceIDs are the advances this receipt DRAWS from — the ids on its
	// PREPAYMENT_PAYMENT rows, which are the source advances' receipt ids.
	AdvanceSourceIDs []string
	// HasExtras is true when the receipt carries extra GL rows (SmartAccounts'
	// `extras`: an accountant's fee or rounding posting on an account). They are
	// not allocations, but deleting the receipt deletes them too.
	HasExtras bool
	// CreatedBy is who made the receipt, as the provider records it.
	// SmartAccounts writes "api" for one made through the API and the user's
	// email for one a person entered (a statement import included); "" when the
	// provider does not say. Use CreatedThroughAPI rather than comparing.
	CreatedBy string
	// DecodeError is non-empty when part of the provider's record could not be
	// read. Such a receipt must never be treated as money; it is returned so the
	// caller can report it rather than lose it.
	DecodeError string

	// The fields below are set by Excellent Books only (eb-receipts); every
	// other provider leaves them zero, so nothing that reads them changes for
	// Merit or SmartAccounts.

	// Draft is true for a receipt the provider holds but has not confirmed
	// (Excellent Books OKFlag != 1). It is not in the books yet: never money,
	// never held, and it may still be deleted.
	Draft bool
	// Void is true for a receipt the provider marks invalidated. It counts as
	// absent.
	Void bool
	// Sequence is the provider's change sequence of this record (Excellent
	// Books @sequence), "" when the provider has none.
	Sequence string
}

// CreatedThroughAPI reports whether the provider says the receipt was made
// through its API. It is evidence that an integration made it, not which one;
// a person-entered receipt (CreatedBy an email) and an unknown one ("") both
// answer false.
func (r Receipt) CreatedThroughAPI() bool {
	return r.CreatedBy == saCreatedByAPI
}

// AmountOn is the sum of the receipt's CLIENT_INVOICE rows on invoiceID.
func (r Receipt) AmountOn(invoiceID string) decimal.Decimal {
	sum := decimal.Zero
	for _, row := range r.Rows {
		if row.Type == saRowClientInvoice && row.DocID == invoiceID {
			sum = sum.Add(row.Amount)
		}
	}
	return sum
}

// TouchesOnly reports whether deleting this receipt would change nothing but
// invoiceID: exactly one row, a CLIENT_INVOICE row on that invoice carrying the
// whole receipt, no advance parked, no extras, nothing unreadable. A provider
// deletes a receipt with all its rows, so anything else would undo money on
// some other document — or an accountant's own posting — too. Extras are checked
// on their own, not left to the amount comparison: extras that sum to zero, or
// an amount that excludes them, would pass it.
func (r Receipt) TouchesOnly(invoiceID string) bool {
	if r.DecodeError != "" || r.HasExtras || len(r.Rows) != 1 || !r.AdvanceAmount.IsZero() || len(r.AdvanceSourceIDs) > 0 {
		return false
	}
	row := r.Rows[0]
	return row.Type == saRowClientInvoice && row.DocID == invoiceID && row.Amount.Equal(r.Amount)
}

// PaymentChanges is one read of the provider's change feed.
type PaymentChanges struct {
	// Receipts are the changed receipts, each once.
	Receipts []Receipt
	// Payments is the same read in the projection ListPaymentsSince returns, for
	// callers that attribute dates and banks the way they always have.
	Payments []Payment
	// Deleted are the ids the provider reports deleted in the window.
	Deleted []string
	// DeletedComplete is false when the provider's answer did not prove its
	// deleted list complete. The caller may act on an id IN Deleted; it must not
	// act on an id's ABSENCE from Deleted unless this is true.
	DeletedComplete bool

	// Sequence is the cursor to send as PaymentChangesInput.SinceSequence next
	// time (Excellent Books: the response's data@sequence). "" when the read
	// was not sequence-based. Store it only after the whole read was acted on.
	Sequence string
}

// saRowClientInvoice mirrors smartaccounts.RowClientInvoice; receipts are a
// root type and the row vocabulary is the only provider whose rows exist today.
const saRowClientInvoice = "CLIENT_INVOICE"

// saCreatedByAPI mirrors smartaccounts.CreatedByAPI.
const saCreatedByAPI = "api"

// ReceiptReverser undoes a confirmed receipt by booking its negative copy —
// for a provider that cannot delete a confirmed receipt (Excellent Books: "a
// confirmed receipt cannot be changed; copy it, put a minus sign in front of the
// received amount, confirm and save"). The original stays in the books; the
// pair nets to zero. The returned CreatedReceipt is the REVERSAL, not the
// original.
type ReceiptReverser interface {
	ReverseReceipt(ctx context.Context, input ReverseReceiptInput) (*CreatedReceipt, error)
}

// ReverseReceiptInput names the receipt to reverse.
type ReverseReceiptInput struct {
	// ID is the provider's id of the receipt to reverse (Excellent Books SerNr).
	ID string
	// Date is the reversal's own date. Zero ⇒ the original's date.
	Date time.Time
	// Document is written where the provider keeps the caller's tag (Excellent
	// Books: the header Comment), so the caller can recognise the reversal as
	// its own. Empty ⇒ not sent.
	Document string
}

// ErrChangeFeedUnsupported is returned by ListPaymentChanges when the caller
// asked for a sequence-based read (SinceSequence) and the provider refused it
// — Excellent Books answers updates_after only on registers with a
// synchronisation index. The caller falls back to a date window.
var ErrChangeFeedUnsupported = errors.New("change feed unsupported for this register")

// --- Service wrappers ---

// SupportsReceiptCreate reports whether the provider implements CreateReceipt.
func (s *PaymentService) SupportsReceiptCreate() bool {
	_, ok := s.provider.(ReceiptCreator)
	return ok
}

// CreateReceipt books a receipt and returns its provider id. Unlike Create it
// honours CreatePaymentInput's receipt fields (AccountKind, Document, Comment,
// ResolvedInvoice, ExpectInvoiceID). ErrUnsupportedProvider when the provider
// lacks it — never a fallback to Create, which would book without them.
func (s *PaymentService) CreateReceipt(ctx context.Context, input CreatePaymentInput) (*CreatedReceipt, error) {
	rc, ok := s.provider.(ReceiptCreator)
	if !ok {
		return nil, &ProviderError{Provider: s.providerName, Op: "CreateReceipt", Err: ErrUnsupportedProvider}
	}
	return rc.CreateReceipt(ctx, input)
}

// SupportsReceiptRead reports whether the provider can read receipts back
// both per customer and by id.
func (s *PaymentService) SupportsReceiptRead() bool {
	_, lister := s.provider.(InvoiceReceiptLister)
	_, getter := s.provider.(ReceiptGetter)
	return lister && getter
}

// ListInvoiceReceipts reads one customer's receipts.
func (s *PaymentService) ListInvoiceReceipts(ctx context.Context, input ListInvoiceReceiptsInput) ([]Receipt, error) {
	l, ok := s.provider.(InvoiceReceiptLister)
	if !ok {
		return nil, &ProviderError{Provider: s.providerName, Op: "ListInvoiceReceipts", Err: ErrUnsupportedProvider}
	}
	return l.ListInvoiceReceipts(ctx, input)
}

// GetReceipt reads one receipt by id; IsNotFound on a receipt that is gone.
func (s *PaymentService) GetReceipt(ctx context.Context, id string) (*Receipt, error) {
	g, ok := s.provider.(ReceiptGetter)
	if !ok {
		return nil, &ProviderError{Provider: s.providerName, Op: "GetReceipt", Err: ErrUnsupportedProvider}
	}
	return g.GetReceipt(ctx, id)
}

// SupportsReceiptReverse reports whether the provider undoes receipts by a
// negative copy (ReceiptReverser).
func (s *PaymentService) SupportsReceiptReverse() bool {
	_, ok := s.provider.(ReceiptReverser)
	return ok
}

// ReverseReceipt books the negative copy of a confirmed receipt.
// ErrUnsupportedProvider when the provider lacks it.
func (s *PaymentService) ReverseReceipt(ctx context.Context, input ReverseReceiptInput) (*CreatedReceipt, error) {
	r, ok := s.provider.(ReceiptReverser)
	if !ok {
		return nil, &ProviderError{Provider: s.providerName, Op: "ReverseReceipt", Err: ErrUnsupportedProvider}
	}
	return r.ReverseReceipt(ctx, input)
}

// SupportsPaymentChanges reports whether the provider has a receipt-level
// change feed.
func (s *SyncService) SupportsPaymentChanges() bool {
	_, ok := s.provider.(PaymentChangeLister)
	return ok
}

// ListPaymentChanges reads every receipt changed or deleted in the window.
func (s *SyncService) ListPaymentChanges(ctx context.Context, input PaymentChangesInput) (*PaymentChanges, error) {
	l, ok := s.provider.(PaymentChangeLister)
	if !ok {
		return nil, &ProviderError{Provider: s.providerName, Op: "ListPaymentChanges", Err: ErrUnsupportedProvider}
	}
	return l.ListPaymentChanges(ctx, input)
}

// SupportsCashAccounts reports whether the provider has a cash-desk register.
func (s *TaxService) SupportsCashAccounts() bool {
	_, ok := s.provider.(CashAccountLister)
	return ok
}

// ListCashAccounts returns the provider's cash desks.
func (s *TaxService) ListCashAccounts(ctx context.Context) ([]Bank, error) {
	l, ok := s.provider.(CashAccountLister)
	if !ok {
		return nil, &ProviderError{Provider: s.providerName, Op: "ListCashAccounts", Err: ErrUnsupportedProvider}
	}
	return l.ListCashAccounts(ctx)
}

// SupportsNettingBanks reports whether the provider marks a netting bank.
func (s *TaxService) SupportsNettingBanks() bool {
	_, ok := s.provider.(NettingBankLister)
	return ok
}

// ListNettingBanks names the provider's netting bank accounts.
func (s *TaxService) ListNettingBanks(ctx context.Context) ([]string, error) {
	l, ok := s.provider.(NettingBankLister)
	if !ok {
		return nil, &ProviderError{Provider: s.providerName, Op: "ListNettingBanks", Err: ErrUnsupportedProvider}
	}
	return l.ListNettingBanks(ctx)
}

// BudgetUsed reports this process's request count against the provider's
// budget window, and whether the provider meters one at all.
func (c *Client) BudgetUsed() (used int, known bool) {
	br, ok := c.provider.(BudgetReporter)
	if !ok {
		return 0, false
	}
	return br.BudgetUsed(), true
}
