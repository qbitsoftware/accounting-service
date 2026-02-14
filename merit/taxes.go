package merit

import "context"

// ListTaxes retrieves the list of tax definitions.
func (c *Client) ListTaxes(ctx context.Context) ([]TaxItem, error) {
	var result []TaxItem
	if err := c.post(ctx, "v1/gettaxes", struct{}{}, &result); err != nil {
		return nil, err
	}
	return result, nil
}
