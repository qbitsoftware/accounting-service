package simplbooks

import (
	"context"
	"fmt"
	"maps"
)

// ListClients returns clients matching the given params (single page).
func (c *Client) ListClients(ctx context.Context, params ListParams) ([]ClientRecord, error) {
	resp, err := c.do(ctx, "GET", "clients/list", params.body())
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Client ClientRecord `json:"Client"`
	}
	if err := resp.decodeData(&rows); err != nil {
		return nil, fmt.Errorf("simplbooks: parse clients: %w", err)
	}
	out := make([]ClientRecord, len(rows))
	for i := range rows {
		out[i] = rows[i].Client
	}
	return out, nil
}

// ListAllClients pages through clients/list until exhausted.
func (c *Client) ListAllClients(ctx context.Context, filter map[string]any, perPage int) ([]ClientRecord, error) {
	if perPage <= 0 {
		perPage = 100
	}
	var all []ClientRecord
	for page := 1; ; page++ {
		batch, err := c.ListClients(ctx, ListParams{Page: page, PerPage: perPage, Filter: filter})
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

// GetClient returns a single client by ID.
func (c *Client) GetClient(ctx context.Context, id string) (*ClientRecord, error) {
	resp, err := c.do(ctx, "GET", "clients/get/"+id, nil)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Client ClientRecord `json:"Client"`
	}
	if err := resp.decodeData(&payload); err != nil {
		return nil, fmt.Errorf("simplbooks: parse client: %w", err)
	}
	return &payload.Client, nil
}

// CreateClient posts a client and returns the new client ID. fields is the
// Client object (name, reg_no, e_mail, ...).
func (c *Client) CreateClient(ctx context.Context, fields map[string]any) (int, error) {
	resp, err := c.do(ctx, "POST", "clients/create", map[string]any{"Client": fields})
	if err != nil {
		return 0, err
	}
	return resp.insertedID()
}

// UpdateClient updates an existing client. The ID travels inside the Client
// object body.
func (c *Client) UpdateClient(ctx context.Context, id int, fields map[string]any) error {
	client := map[string]any{"id": id}
	maps.Copy(client, fields)
	_, err := c.do(ctx, "POST", "clients/update", map[string]any{"Client": client})
	return err
}
