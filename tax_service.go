package accounting

import "context"

type TaxService struct {
	provider Provider
}

func (s *TaxService) List(ctx context.Context) ([]Tax, error) {
	return s.provider.ListTaxes(ctx)
}

// ListAccounts returns the chart of accounts (GL accounts) from the provider.
func (s *TaxService) ListAccounts(ctx context.Context) ([]Account, error) {
	return s.provider.ListAccounts(ctx)
}
