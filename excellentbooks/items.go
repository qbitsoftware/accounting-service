package excellentbooks

import (
	"context"
	"encoding/json"
	"fmt"
)

const registerItem = "INVc"

// ListItems retrieves items/articles.
func (c *Client) ListItems(ctx context.Context, params ListParams) ([]Item, string, error) {
	resp, err := c.get(ctx, registerItem, params)
	if err != nil {
		return nil, "", err
	}
	return parseItemResponse(resp)
}

// GetItem retrieves a single item by code.
func (c *Client) GetItem(ctx context.Context, code string) (*Item, error) {
	resp, err := c.getOne(ctx, registerItem, code)
	if err != nil {
		return nil, err
	}

	items, _, err := parseItemResponse(resp)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, &APIError{StatusCode: 404, Message: "item not found"}
	}
	return &items[0], nil
}

// CreateItem creates a new item.
func (c *Client) CreateItem(ctx context.Context, fields map[string]string) (*Item, error) {
	resp, err := c.post(ctx, registerItem, fields)
	if err != nil {
		return nil, err
	}

	items, _, err := parseItemResponse(resp)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("excellentbooks: no item returned after create")
	}
	return &items[0], nil
}

func parseItemResponse(resp *Response) ([]Item, string, error) {
	var envelope struct {
		ResponseMeta
		INVc []Item `json:"INVc"`
	}
	if err := json.Unmarshal(resp.Data, &envelope); err != nil {
		return nil, "", fmt.Errorf("excellentbooks: parse items: %w", err)
	}
	return envelope.INVc, envelope.Sequence, nil
}
