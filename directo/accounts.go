package directo

import (
	"context"
	"net/url"
)

// ListAccounts retrieves GL accounts via REST API.
func (c *Client) ListAccounts(ctx context.Context) ([]AccountREST, error) {
	var result []AccountREST
	err := c.rest.get(ctx, "accounts", nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ListObjects retrieves objects/dimensions via REST API.
func (c *Client) ListObjects(ctx context.Context) ([]ObjectREST, error) {
	var result []ObjectREST
	err := c.rest.get(ctx, "objects", nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ListProjects retrieves projects via REST API.
func (c *Client) ListProjects(ctx context.Context) ([]ProjectREST, error) {
	var result []ProjectREST
	err := c.rest.get(ctx, "projects", nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ListDeletedRecords retrieves deleted records since a timestamp.
func (c *Client) ListDeletedRecords(ctx context.Context, tsFrom string) ([]map[string]any, error) {
	params := url.Values{}
	if tsFrom != "" {
		params.Set("ts", ">"+tsFrom)
	}
	var result []map[string]any
	err := c.rest.get(ctx, "deleted", params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
