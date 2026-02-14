package merit

import "context"

// ListCustomers retrieves a list of customers matching the given criteria.
func (c *Client) ListCustomers(ctx context.Context, params ListCustomersParams) ([]CustomerListItem, error) {
	var result []CustomerListItem
	if err := c.post(ctx, "v1/getcustomers", params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateCustomer creates a new customer.
func (c *Client) CreateCustomer(ctx context.Context, req CreateCustomerRequest) (*CreateCustomerResponse, error) {
	var result CreateCustomerResponse
	if err := c.post(ctx, "v2/sendcustomer", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateCustomer updates an existing customer.
func (c *Client) UpdateCustomer(ctx context.Context, req UpdateCustomerRequest) error {
	return c.post(ctx, "v1/updatecustomer", req, nil)
}
