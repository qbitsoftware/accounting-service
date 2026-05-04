package merit

import "context"

// BankItem represents a bank account configured in Merit Aktiva, as returned
// by the v1/getbanks endpoint.
type BankItem struct {
	BankID       string `json:"BankId"`
	Name         string `json:"Name"`
	Description  string `json:"Description"`
	IBANCode     string `json:"IBANCode"`
	AccountCode  string `json:"AccountCode"`
	CurrencyCode string `json:"CurrencyCode"`
}

// ListBanks retrieves the bank accounts configured in Merit.
func (c *Client) ListBanks(ctx context.Context) ([]BankItem, error) {
	var result []BankItem
	if err := c.post(ctx, "v1/getbanks", struct{}{}, &result); err != nil {
		return nil, err
	}
	return result, nil
}
