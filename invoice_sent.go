package accounting

import "context"

// InvoiceSentMarker is an optional capability for accounting backends that track
// a "sent to customer" state on the invoice document itself. Like
// InvoiceDateEditor it is deliberately NOT part of the core Provider interface:
// callers type-assert for it, so providers with no such state need not
// implement it.
//
// Only SimplBooks implements this today (POST invoices/sent/{id}). Merit,
// Excellent Books, SmartAccounts and Directo expose no equivalent flag on the
// sales-invoice API.
//
// Prefer CreateInvoiceInput.SentDate for a fresh push — it stamps the real send
// date in the same request. This interface exists for the repair case: invoices
// already pushed without a sent date, which the provider therefore shows as
// unsent even though the caller mailed them long ago. The provider stamps its
// own current date, not the original one.
type InvoiceSentMarker interface {
	// MarkInvoiceSent flags the invoice identified by the provider-side ID as
	// sent, leaving every other field untouched.
	MarkInvoiceSent(ctx context.Context, id string) error
}
