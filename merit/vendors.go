package merit

import "context"

// ListVendors retrieves a list of vendors matching the given criteria.
func (c *Client) ListVendors(ctx context.Context, params ListVendorsParams) ([]VendorListItem, error) {
	var result []VendorListItem
	if err := c.post(ctx, "v1/getvendors", params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateVendor creates a new vendor.
func (c *Client) CreateVendor(ctx context.Context, req CreateVendorRequest) (*CreateVendorResponse, error) {
	var result CreateVendorResponse
	if err := c.post(ctx, "v2/sendvendor", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateVendor updates an existing vendor.
func (c *Client) UpdateVendor(ctx context.Context, req UpdateVendorRequest) error {
	return c.post(ctx, "v2/updatevendor", req, nil)
}
