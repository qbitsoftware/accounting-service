package accounting

import (
	"context"
	"sync"
)

type InvoiceService struct {
	provider     Provider
	providerName string
}

// SupportsDateEdit reports whether the configured provider can change an
// already-issued invoice's dates (the optional InvoiceDateEditor capability).
func (s *InvoiceService) SupportsDateEdit() bool {
	_, ok := s.provider.(InvoiceDateEditor)
	return ok
}

// UpdateDates changes an invoice's document/due date in place. Returns
// ErrUnsupportedProvider (wrapped with the provider name) when the provider
// does not implement InvoiceDateEditor.
func (s *InvoiceService) UpdateDates(ctx context.Context, input UpdateInvoiceDatesInput) error {
	de, ok := s.provider.(InvoiceDateEditor)
	if !ok {
		return &ProviderError{Provider: s.providerName, Op: "UpdateInvoiceDates", Err: ErrUnsupportedProvider}
	}
	return de.UpdateInvoiceDates(ctx, input)
}

// SupportsMarkSent reports whether the configured provider tracks a
// sent-to-customer flag on the invoice (the optional InvoiceSentMarker
// capability).
func (s *InvoiceService) SupportsMarkSent() bool {
	_, ok := s.provider.(InvoiceSentMarker)
	return ok
}

// MarkSent flags an already-pushed invoice as sent. Returns
// ErrUnsupportedProvider (wrapped with the provider name) when the provider does
// not implement InvoiceSentMarker. For a fresh push set
// CreateInvoiceInput.SentDate instead — it carries the true send date and costs
// no extra request.
func (s *InvoiceService) MarkSent(ctx context.Context, id string) error {
	sm, ok := s.provider.(InvoiceSentMarker)
	if !ok {
		return &ProviderError{Provider: s.providerName, Op: "MarkInvoiceSent", Err: ErrUnsupportedProvider}
	}
	return sm.MarkInvoiceSent(ctx, id)
}

func (s *InvoiceService) Create(ctx context.Context, input CreateInvoiceInput) (*Invoice, error) {
	return s.provider.CreateInvoice(ctx, input)
}

func (s *InvoiceService) Get(ctx context.Context, id string) (*Invoice, error) {
	return s.provider.GetInvoice(ctx, id)
}

func (s *InvoiceService) GetPDF(ctx context.Context, id string, deliveryNote bool) (*InvoicePDF, error) {
	return s.provider.GetInvoicePDF(ctx, id, deliveryNote)
}

func (s *InvoiceService) List(ctx context.Context, input ListInvoicesInput) ([]Invoice, error) {
	return s.provider.ListInvoices(ctx, input)
}

func (s *InvoiceService) Delete(ctx context.Context, id string) error {
	return s.provider.DeleteInvoice(ctx, id)
}

func (s *InvoiceService) FindByRef(ctx context.Context, refStr string) (*Invoice, error) {
	return s.provider.FindInvoiceByRef(ctx, refStr)
}

func (s *InvoiceService) CreateCreditNote(ctx context.Context, input CreateCreditNoteInput) (*Invoice, error) {
	return s.provider.CreateCreditNote(ctx, input)
}

func (s *InvoiceService) BatchCreate(ctx context.Context, inputs []CreateInvoiceInput) []BatchResult {
	results := make([]BatchResult, len(inputs))
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for i, input := range inputs {
		results[i].Index = i
		wg.Add(1)
		go func(idx int, in CreateInvoiceInput) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			inv, err := s.provider.CreateInvoice(ctx, in)
			results[idx].Invoice = inv
			results[idx].Err = err
		}(i, input)
	}

	wg.Wait()
	return results
}
