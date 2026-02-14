package accounting

import "context"

type PurchaseService struct {
	provider Provider
}

func (s *PurchaseService) Create(ctx context.Context, input CreatePurchaseInput) (*PurchaseInvoice, error) {
	return s.provider.CreatePurchase(ctx, input)
}

func (s *PurchaseService) Get(ctx context.Context, id string) (*PurchaseInvoice, error) {
	return s.provider.GetPurchase(ctx, id)
}

func (s *PurchaseService) List(ctx context.Context, input ListPurchasesInput) ([]PurchaseInvoice, error) {
	return s.provider.ListPurchases(ctx, input)
}

func (s *PurchaseService) Delete(ctx context.Context, id string) error {
	return s.provider.DeletePurchase(ctx, id)
}
