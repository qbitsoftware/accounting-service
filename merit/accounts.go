package merit

import "context"

// ListAccounts retrieves the chart of accounts.
func (c *Client) ListAccounts(ctx context.Context) ([]AccountItem, error) {
	var result []AccountItem
	if err := c.post(ctx, "v1/getaccounts", struct{}{}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListProjects retrieves the list of projects.
func (c *Client) ListProjects(ctx context.Context) ([]ProjectItem, error) {
	var result []ProjectItem
	if err := c.post(ctx, "v1/getprojects", struct{}{}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListCostCenters retrieves the list of cost centers.
func (c *Client) ListCostCenters(ctx context.Context) ([]CostCenterItem, error) {
	var result []CostCenterItem
	if err := c.post(ctx, "v1/getcostcenters", struct{}{}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListDepartments retrieves the list of departments.
func (c *Client) ListDepartments(ctx context.Context) ([]Department, error) {
	var result []Department
	if err := c.post(ctx, "v1/getdepartments", struct{}{}, &result); err != nil {
		return nil, err
	}
	return result, nil
}
