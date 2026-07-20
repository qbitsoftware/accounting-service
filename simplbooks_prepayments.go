package accounting

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/qbitsoftware/accounting-service/simplbooks"
	"github.com/shopspring/decimal"
)

// SimplBooks customer-prepayment (ettemaks) support.
//
// SimplBooks has no dedicated prepayment register, so an advance is modeled on
// the closest primitive: an Incoming (customer receipt) that is NOT bound to any
// invoice — money received on account. Applying it later binds that incoming to
// an invoice via incomings/invoice_paid.
//
//	PARK  — incomings/create with client_id + income_sum + a bank
//	        (income_account_id) and NO invoice_id. SimplBooks assigns the
//	        incoming id, which becomes the prepayment's stable handle
//	        (Number/DocID), like SmartAccounts. The description is tagged so the
//	        advance can be told apart from ordinary invoice receipts on list.
//	APPLY — incomings/invoice_paid(incoming_id, invoice_id): binds the parked
//	        advance to an invoice, settling it when the sum covers the balance.
//	LIST  — incomings/list filtered to the tag, per customer.
//
// UnallocateToPrepayment is intentionally unsupported: SimplBooks exposes no
// un-bind endpoint, and — unlike Excellent Books — its credit notes are not
// gated on the invoice's still-open amount (a credit note is just a new invoice
// carrying credit_invoice_for), so the EB "free the paid amount first" step is
// unnecessary. This mirrors the SmartAccounts decision.
//
// BEHAVIOUR, verified live against a Premium tenant 2026-07-20:
//   - An advance draws down ACROSS invoices. Binding one 50 EUR advance to a
//     30 EUR invoice and then to a 20 EUR invoice settles both fully; each bind
//     applies min(advance remaining, invoice remnant). This is the same
//     progressive-drawdown model as SmartAccounts and Excellent Books.
//   - Partial settlement works in both directions: an advance smaller than the
//     invoice leaves the invoice partly paid (10 EUR onto a 40 EUR invoice →
//     remnant 30).
//   - Because each bind applies min(remaining, remnant), a caller consuming
//     lots FIFO gets the same allocation SimplBooks computes — so the missing
//     amount parameter is not a practical limitation. It only bites if a caller
//     wants to apply LESS than min(lot remaining, invoice remnant), which the
//     API cannot express.
//   - Binding an exhausted advance is a SILENT no-op that still answers HTTP
//     200 / data:true; BindIncomingToInvoice converts it to ErrAlreadyAllocated.
//
// REMAINING LIMITATION: SimplBooks exposes no per-receipt remaining AMOUNT —
// only the `bound` boolean (false = something left, true = fully consumed).
// ListPrepayments therefore reports Remaining as the full sum while unbound and
// zero once bound; it cannot report a partially-drawn balance. Callers that need
// exact figures must track lots themselves (klubio does) or reconcile via the
// invoices' paid/remnant fields.
//
// simplbooksProvider satisfies PrepaymentProvider.
var _ PrepaymentProvider = (*simplbooksProvider)(nil)

// simplbooksPrepaymentTag marks an Incoming as a customer advance so it can be
// distinguished from ordinary invoice receipts, which SimplBooks stores in the
// same register with no type flag.
const simplbooksPrepaymentTag = "[ETTEMAKS]"

// CreatePrepayment parks an unallocated customer advance as an unbound Incoming.
func (p *simplbooksProvider) CreatePrepayment(ctx context.Context, input CreatePrepaymentInput) (*Prepayment, error) {
	bankID := atoiOrZero(input.BankID)
	if bankID == 0 {
		return nil, p.wrapError("CreatePrepayment", fmt.Errorf("%w: BankID (income_account_id) is required", ErrInvalidInput))
	}
	clientID := atoiOrZero(input.CustomerCode)
	if clientID == 0 {
		return nil, p.wrapError("CreatePrepayment", fmt.Errorf("%w: CustomerCode (client_id) is required", ErrInvalidInput))
	}
	if !input.Amount.IsPositive() {
		return nil, p.wrapError("CreatePrepayment", fmt.Errorf("%w: amount must be positive", ErrInvalidInput))
	}

	fields := map[string]any{
		"income_account_id": bankID,
		"income_sum":        input.Amount.String(),
		"income_date":       formatSimplBooksDate(input.PaymentDate),
		"client_id":         clientID,
		"description":       simplbooksPrepaymentDescription(input.PrepaymentNo, input.Comment),
	}
	if input.Currency != "" {
		fields["currency_name"] = input.Currency
	}

	id, err := p.client.CreateIncoming(ctx, fields, 0) // 0 => unbound advance
	if err != nil {
		return nil, p.wrapError("CreatePrepayment", err)
	}
	return &Prepayment{
		Number:       strconv.Itoa(id), // SimplBooks-assigned incoming id is the handle
		DocID:        strconv.Itoa(id),
		CustomerCode: input.CustomerCode,
		Amount:       input.Amount,
		Remaining:    input.Amount,
		Currency:     input.Currency,
		Date:         input.PaymentDate,
		Comment:      input.Comment,
	}, nil
}

// ApplyPrepayment binds a parked advance to an invoice. input.PrepaymentNo is
// the SimplBooks incoming id (from CreatePrepayment); input.InvoiceNo is the
// SimplBooks invoice id. See the file-level LIMITATIONS on partial amounts.
func (p *simplbooksProvider) ApplyPrepayment(ctx context.Context, input ApplyPrepaymentInput) error {
	incomingID := atoiOrZero(input.PrepaymentNo)
	invoiceID := atoiOrZero(input.InvoiceNo)
	if incomingID == 0 || invoiceID == 0 {
		return p.wrapError("ApplyPrepayment", fmt.Errorf("%w: PrepaymentNo (incoming id) and InvoiceNo (invoice id) are required", ErrInvalidInput))
	}
	// full_cover=false: bind and let SimplBooks settle by the receipt sum rather
	// than forcing full coverage (which could write off an uncovered remainder).
	return p.wrapError("ApplyPrepayment", p.client.BindIncomingToInvoice(ctx, incomingID, invoiceID, false))
}

// UnallocateToPrepayment is unsupported on SimplBooks (no un-bind API; credit
// notes are not gated on open amount, so it is unnecessary). See file header.
func (p *simplbooksProvider) UnallocateToPrepayment(_ context.Context, _ UnallocateToPrepaymentInput) (*Prepayment, error) {
	return nil, p.wrapError("UnallocateToPrepayment", ErrUnsupportedProvider)
}

// ListPrepayments returns tagged advance receipts, optionally scoped to a
// customer and date window. See the file-level LIMITATIONS on Remaining.
func (p *simplbooksProvider) ListPrepayments(ctx context.Context, input ListPrepaymentsInput) ([]Prepayment, error) {
	// incomings/list has no client_id filter; scope by the tag server-side
	// (description "contains") and by customer/date client-side.
	incs, err := p.client.ListAllIncomings(ctx, map[string]any{"description": simplbooksPrepaymentTag}, 200)
	if err != nil {
		return nil, p.wrapError("ListPrepayments", err)
	}
	return simplbooksPrepaymentsFromIncomings(incs, input.CustomerCode, input.Since, input.Until), nil
}

// simplbooksPrepaymentDescription builds the receipt description, embedding the
// tag (always) plus the caller's prepayment number and comment for traceability.
func simplbooksPrepaymentDescription(prepaymentNo, comment string) string {
	parts := []string{simplbooksPrepaymentTag}
	if prepaymentNo != "" {
		parts = append(parts, prepaymentNo)
	}
	if comment != "" {
		parts = append(parts, comment)
	}
	return strings.Join(parts, " ")
}

// simplbooksPrepaymentsFromIncomings maps tagged advance receipts to Prepayments.
// Pure, so the tag/customer/date filtering is unit-tested without the network.
// Remaining is derived from the `bound` flag — the only allocation signal
// SimplBooks exposes — so it is all-or-nothing, never a partial balance
// (see the file header).
func simplbooksPrepaymentsFromIncomings(incs []simplbooks.Incoming, customerCode string, since, until time.Time) []Prepayment {
	wantClient := atoiOrZero(customerCode)
	var out []Prepayment
	for i := range incs {
		inc := incs[i]
		if !strings.Contains(inc.Description, simplbooksPrepaymentTag) {
			continue
		}
		if wantClient != 0 && inc.ClientID != wantClient {
			continue
		}
		date := parseSimplBooksDate(inc.IncomeDate)
		if !inDateRange(date, since, until) {
			continue
		}
		remaining := inc.IncomeSum.Decimal
		if inc.Bound {
			remaining = decimal.Zero
		}
		out = append(out, Prepayment{
			Number:       strconv.Itoa(inc.ID),
			DocID:        strconv.Itoa(inc.ID),
			CustomerCode: itoaOrEmpty(inc.ClientID),
			Amount:       inc.IncomeSum.Decimal,
			Remaining:    remaining,
			Currency:     inc.CurrencyName,
			Date:         date,
			Comment:      inc.Description,
		})
	}
	return out
}
