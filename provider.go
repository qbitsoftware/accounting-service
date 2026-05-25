package accounting

import (
	"context"
	"time"
)

// Provider defines the interface that accounting backends must implement.
type Provider interface {
	TestConnection(ctx context.Context) error

	// Invoices
	CreateInvoice(ctx context.Context, input CreateInvoiceInput) (*Invoice, error)
	GetInvoice(ctx context.Context, id string) (*Invoice, error)
	GetInvoicePDF(ctx context.Context, id string, deliveryNote bool) (*InvoicePDF, error)
	ListInvoices(ctx context.Context, input ListInvoicesInput) ([]Invoice, error)
	FindInvoiceByRef(ctx context.Context, refStr string) (*Invoice, error)
	DeleteInvoice(ctx context.Context, id string) error
	// SendAsEInvoice transmits an already-created provider-side invoice
	// as an e-invoice (e-arve) over the operator network. The recipient's
	// delivery preferences are read from the provider-side customer
	// record (set on CreateCustomer/UpdateCustomer). Pass the ID returned
	// by CreateInvoice or CreateCreditNote.
	//
	// Errors:
	//   - ErrEInvoiceNotSupported → recipient is not e-invoice capable
	//   - ErrFeatureNotSupported  → this Provider cannot send e-invoices
	//
	// Capability flag: Capabilities.SupportsEInvoiceSend.
	SendAsEInvoice(ctx context.Context, providerInvoiceID string, deliveryNote bool) error

	// Customers
	CreateCustomer(ctx context.Context, input CreateCustomerInput) (*Customer, error)
	UpdateCustomer(ctx context.Context, input UpdateCustomerInput) error
	ListCustomers(ctx context.Context, input ListCustomersInput) ([]Customer, error)
	FindCustomerByEmail(ctx context.Context, email string) (*Customer, error)
	// GetCustomer fetches a single customer card by its provider-side ID/code.
	// Supported by Excellent Books (code-keyed register); Merit/Directo return
	// a "not supported" error.
	GetCustomer(ctx context.Context, id string) (*Customer, error)

	// Payments
	CreatePayment(ctx context.Context, input CreatePaymentInput) error
	ListPayments(ctx context.Context, input ListPaymentsInput) ([]Payment, error)
	DeletePayment(ctx context.Context, id string) error

	// Items
	CreateItem(ctx context.Context, input CreateItemInput) (*Item, error)
	ListItems(ctx context.Context, input ListItemsInput) ([]Item, error)
	UpdateItem(ctx context.Context, input UpdateItemInput) error

	// Credit Notes
	CreateCreditNote(ctx context.Context, input CreateCreditNoteInput) (*Invoice, error)

	// Purchases
	CreatePurchase(ctx context.Context, input CreatePurchaseInput) (*PurchaseInvoice, error)
	GetPurchase(ctx context.Context, id string) (*PurchaseInvoice, error)
	ListPurchases(ctx context.Context, input ListPurchasesInput) ([]PurchaseInvoice, error)
	DeletePurchase(ctx context.Context, id string) error

	// Reference data
	ListTaxes(ctx context.Context) ([]Tax, error)
	ListAccounts(ctx context.Context) ([]Account, error)
	ListDimensions(ctx context.Context) (*DimensionList, error)
	// ListBanks returns the bank accounts configured in the accounting
	// system. Used to seed the bank-account mapping UI. Providers that do
	// not expose a banks register (e.g. Excellent Books) return an empty
	// slice.
	ListBanks(ctx context.Context) ([]Bank, error)
	// ListPaymentTerms returns the payment-term codes the provider has
	// configured (e.g. "K" for cash, "P14" for 14-day net). Used to populate
	// the credit-note creation dropdown. Providers without a payment-term
	// register (Merit) return an empty slice.
	ListPaymentTerms(ctx context.Context) ([]PaymentTerm, error)

	// Reports
	CustomerDebts(ctx context.Context, customerName string, overdueDays *int) ([]CustomerDebt, error)

	// Sync
	ListInvoicesSince(ctx context.Context, since time.Time, until time.Time) ([]Invoice, error)
	ListPaymentsSince(ctx context.Context, since time.Time, until time.Time) ([]Payment, error)
}
