package merit

import "context"

// CustomerDebts retrieves the customer debts report.
func (c *Client) CustomerDebts(ctx context.Context, params CustomerDebtsParams) ([]CustomerDebtItem, error) {
	var result []CustomerDebtItem
	if err := c.post(ctx, "v1/getcustdebtrep", params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ProfitLoss retrieves a profit and loss (income statement) report.
func (c *Client) ProfitLoss(ctx context.Context, params ProfitLossParams) (*FinancialReport, error) {
	var result FinancialReport
	if err := c.post(ctx, "v1/getprofitrep", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// BalanceSheet retrieves a balance sheet (statement of financial position) report.
func (c *Client) BalanceSheet(ctx context.Context, params BalanceSheetParams) (*FinancialReport, error) {
	var result FinancialReport
	if err := c.post(ctx, "v1/getbalancerep", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
