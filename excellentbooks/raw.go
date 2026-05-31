package excellentbooks

import (
	"context"
	"encoding/json"
)

// GetRaw fetches a single record by ID from the given register and returns the
// raw `data` payload, undecoded. Use it for diagnostics — inspecting fields that
// the typed structs do not (yet) map, e.g. an invoice's server-side open/paid
// amount or a customer's prepayment balance, which never reach mapExcellentInvoice.
func (c *Client) GetRaw(ctx context.Context, register, id string) (json.RawMessage, error) {
	resp, err := c.getOne(ctx, register, id)
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// ListRaw lists records from the given register and returns the raw `data`
// payload, undecoded. Companion to GetRaw for diagnostics.
func (c *Client) ListRaw(ctx context.Context, register string, params ListParams) (json.RawMessage, error) {
	resp, err := c.get(ctx, register, params)
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}
