package smartaccounts

import (
	"context"
	"net/url"
)

// ListClientsParams filters a clients:get query.
type ListClientsParams struct {
	NameOrRegCode string
	ID            string
	ModifiedFrom  string // dd.MM.yyyy
	ModifiedTo    string // dd.MM.yyyy
	FetchContacts bool
	FetchAddress  bool
}

func (p ListClientsParams) values() url.Values {
	v := url.Values{}
	setNonEmpty(v, "nameOrRegCode", p.NameOrRegCode)
	setNonEmpty(v, "id", p.ID)
	setNonEmpty(v, "modifiedFrom", p.ModifiedFrom)
	setNonEmpty(v, "modifiedTo", p.ModifiedTo)
	if p.FetchContacts {
		v.Set("fetchContacts", "true")
	}
	if p.FetchAddress {
		v.Set("fetchAddresses", "true")
	}
	return v
}

// ListClients retrieves clients matching params, following pagination.
func (c *Client) ListClients(ctx context.Context, params ListClientsParams) ([]ClientItem, error) {
	// Always fetch contacts/addresses so email/phone/address are populated.
	params.FetchContacts = true
	params.FetchAddress = true
	var items []ClientItem
	if _, err := c.getList(ctx, "purchasesales/clients:get", params.values(), &items); err != nil {
		return nil, err
	}
	return items, nil
}

// GetClient fetches a single client by ID.
func (c *Client) GetClient(ctx context.Context, id string) (*ClientItem, error) {
	items, err := c.ListClients(ctx, ListClientsParams{ID: id})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, &APIError{StatusCode: 404, Message: "client not found: " + id}
	}
	return &items[0], nil
}

// CreateClient adds a client.
func (c *Client) CreateClient(ctx context.Context, req CreateClientRequest) (*ClientResponse, error) {
	var resp ClientResponse
	if err := c.post(ctx, "purchasesales/clients:add", nil, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// EditClient updates a client (req.ID must be set).
func (c *Client) EditClient(ctx context.Context, req CreateClientRequest) error {
	return c.post(ctx, "purchasesales/clients:edit", nil, req, nil)
}
