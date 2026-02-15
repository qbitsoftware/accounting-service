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
	DeleteInvoice(ctx context.Context, id string) error

	// Customers
	CreateCustomer(ctx context.Context, input CreateCustomerInput) (*Customer, error)
	UpdateCustomer(ctx context.Context, input UpdateCustomerInput) error
	ListCustomers(ctx context.Context, input ListCustomersInput) ([]Customer, error)
	FindCustomerByEmail(ctx context.Context, email string) (*Customer, error)

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

	// Reports
	CustomerDebts(ctx context.Context, customerName string, overdueDays *int) ([]CustomerDebt, error)

	// Sync
	ListInvoicesSince(ctx context.Context, since time.Time, until time.Time) ([]Invoice, error)
	ListPaymentsSince(ctx context.Context, since time.Time, until time.Time) ([]Payment, error)
}
