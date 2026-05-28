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

// ListDimensions returns available projects, cost centers, and departments from the provider.
func (s *TaxService) ListDimensions(ctx context.Context) (*DimensionList, error) {
	return s.provider.ListDimensions(ctx)
}

// ListBanks returns the bank accounts configured in the provider's register
// (e.g. SmartAccounts settings/bankaccounts, Merit /getbanks). Providers with
// no bank register (Excellent Books) return an empty slice.
func (s *TaxService) ListBanks(ctx context.Context) ([]Bank, error) {
	return s.provider.ListBanks(ctx)
}

// ListPaymentTerms returns the payment-term codes configured in the
// provider's register (e.g. Excellent Books "K" for cash). Providers without
// a payment-term register (Merit) return an empty slice.
func (s *TaxService) ListPaymentTerms(ctx context.Context) ([]PaymentTerm, error) {
	return s.provider.ListPaymentTerms(ctx)
}
