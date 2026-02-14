package merit

import "context"

// ListItems retrieves a list of items/products matching the given criteria.
func (c *Client) ListItems(ctx context.Context, params ListItemsParams) ([]ItemListItem, error) {
	var result []ItemListItem
	if err := c.post(ctx, "v1/getitems", params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateItems creates one or more new items.
func (c *Client) CreateItems(ctx context.Context, items []CreateItemRequest) ([]CreateItemResponse, error) {
	var result []CreateItemResponse
	if err := c.post(ctx, "v2/senditems", CreateItemsWrapper{Items: items}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// UpdateItem updates an existing item.
func (c *Client) UpdateItem(ctx context.Context, req UpdateItemRequest) error {
	return c.post(ctx, "v1/updateitem", req, nil)
}
