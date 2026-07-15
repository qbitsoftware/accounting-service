package accounting

import (
	"context"
	"time"
)

// InvoiceDateEditor is an optional capability for accounting backends that
// support changing an already-issued invoice's document/due date in place. It
// is intentionally NOT part of the core Provider interface: callers type-assert
// for it (`de, ok := provider.(InvoiceDateEditor)`), so providers without a
// safe date-edit path need not implement it.
//
// Only SmartAccounts implements this today. SmartAccounts' clientinvoices:edit
// accepts a full-overwrite date change on both unpaid and fully-paid invoices
// (verified against a live tenant); the adapter fetches the invoice and
// re-posts it with the new dates. Providers whose API locks dates once an
// invoice is confirmed/paid (or closed in a VAT period) should NOT implement
// this — leaving it unimplemented makes InvoiceService.UpdateDates return
// ErrUnsupportedProvider, which the caller surfaces as "not supported".
type InvoiceDateEditor interface {
	// UpdateInvoiceDates changes the document date and/or due date of the
	// invoice identified by input.ID, leaving all other fields untouched. A zero
	// DocDate or DueDate leaves that field at its current value.
	UpdateInvoiceDates(ctx context.Context, input UpdateInvoiceDatesInput) error
}

// UpdateInvoiceDatesInput identifies an invoice and the new dates to set. A zero
// DocDate or DueDate means "leave unchanged".
type UpdateInvoiceDatesInput struct {
	ID      string    // provider invoice id (SmartAccounts clientinvoice id)
	DocDate time.Time // new document date; zero = leave unchanged
	DueDate time.Time // new due date; zero = leave unchanged
}
