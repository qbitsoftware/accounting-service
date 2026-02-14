package accounting

import "context"

type TaxService struct {
	provider Provider
}

func (s *TaxService) List(ctx context.Context) ([]Tax, error) {
	return s.provider.ListTaxes(ctx)
}
