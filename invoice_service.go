package accounting

import (
	"context"
	"sync"
)

type InvoiceService struct {
	provider Provider
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
