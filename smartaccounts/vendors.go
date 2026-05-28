package smartaccounts

import (
	"context"
	"net/url"
)

// ListVendorsParams filters a vendors:get query.
type ListVendorsParams struct {
	NameOrRegCode string
	ID            string
	ModifiedFrom  string // dd.MM.yyyy
	ModifiedTo    string // dd.MM.yyyy
}

func (p ListVendorsParams) values() url.Values {
	v := url.Values{}
	setNonEmpty(v, "nameOrRegCode", p.NameOrRegCode)
	setNonEmpty(v, "id", p.ID)
	setNonEmpty(v, "modifiedFrom", p.ModifiedFrom)
	setNonEmpty(v, "modifiedTo", p.ModifiedTo)
	return v
}

// ListVendors retrieves vendors matching params, following pagination.
func (c *Client) ListVendors(ctx context.Context, params ListVendorsParams) ([]VendorItem, error) {
	v := params.values()
	v.Set("fetchContacts", "true")
	v.Set("fetchAddresses", "true")
	var items []VendorItem
	if _, err := c.getList(ctx, "purchasesales/vendors:get", v, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// CreateVendor adds a vendor.
func (c *Client) CreateVendor(ctx context.Context, req CreateVendorRequest) (*VendorResponse, error) {
	var resp VendorResponse
	if err := c.post(ctx, "purchasesales/vendors:add", nil, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// EditVendor updates a vendor (req.ID must be set).
func (c *Client) EditVendor(ctx context.Context, req CreateVendorRequest) error {
	return c.post(ctx, "purchasesales/vendors:edit", nil, req, nil)
}
