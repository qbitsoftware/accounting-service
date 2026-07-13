package simplbooks

import (
	"context"
	"encoding/json"
	"fmt"
)

// ListVatTypes returns the configured VAT types.
func (c *Client) ListVatTypes(ctx context.Context) ([]VatType, error) {
	resp, err := c.do(ctx, "GET", "vat_types/list", nil)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		VatType VatType `json:"VatType"`
	}
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, fmt.Errorf("simplbooks: parse vat types: %w", err)
	}
	out := make([]VatType, len(rows))
	for i := range rows {
		out[i] = rows[i].VatType
	}
	return out, nil
}

// ListFinancialAccounts returns the chart-of-accounts (GL) entries.
func (c *Client) ListFinancialAccounts(ctx context.Context, params ListParams) ([]FinancialAccount, error) {
	resp, err := c.do(ctx, "GET", "financial_accounts/list", params.body())
	if err != nil {
		return nil, err
	}
	var rows []struct {
		FinancialAccount FinancialAccount `json:"FinancialAccount"`
	}
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, fmt.Errorf("simplbooks: parse financial accounts: %w", err)
	}
	out := make([]FinancialAccount, len(rows))
	for i := range rows {
		out[i] = rows[i].FinancialAccount
	}
	return out, nil
}

// ListAllFinancialAccounts pages through financial_accounts/list until exhausted.
func (c *Client) ListAllFinancialAccounts(ctx context.Context, perPage int) ([]FinancialAccount, error) {
	if perPage <= 0 {
		perPage = 200
	}
	var all []FinancialAccount
	for page := 1; ; page++ {
		batch, err := c.ListFinancialAccounts(ctx, ListParams{Page: page, PerPage: perPage})
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < perPage {
			break
		}
	}
	return all, nil
}

// ListIncomeAccounts returns bank accounts and cash registers.
func (c *Client) ListIncomeAccounts(ctx context.Context, params ListParams) ([]IncomeAccount, error) {
	resp, err := c.do(ctx, "GET", "income_accounts/list", params.body())
	if err != nil {
		return nil, err
	}
	var rows []struct {
		IncomeAccount IncomeAccount `json:"IncomeAccount"`
	}
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, fmt.Errorf("simplbooks: parse income accounts: %w", err)
	}
	out := make([]IncomeAccount, len(rows))
	for i := range rows {
		out[i] = rows[i].IncomeAccount
	}
	return out, nil
}

// ListAllIncomeAccounts pages through income_accounts/list until exhausted.
func (c *Client) ListAllIncomeAccounts(ctx context.Context, perPage int) ([]IncomeAccount, error) {
	if perPage <= 0 {
		perPage = 200
	}
	var all []IncomeAccount
	for page := 1; ; page++ {
		batch, err := c.ListIncomeAccounts(ctx, ListParams{Page: page, PerPage: perPage})
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < perPage {
			break
		}
	}
	return all, nil
}
