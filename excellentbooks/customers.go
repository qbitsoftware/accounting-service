package excellentbooks

import (
	"context"
	"encoding/json"
	"fmt"
)

const registerCustomer = "CUVc"

// ListCustomers retrieves contacts/customers.
func (c *Client) ListCustomers(ctx context.Context, params ListParams) ([]Customer, string, error) {
	resp, err := c.get(ctx, registerCustomer, params)
	if err != nil {
		return nil, "", err
	}
	return parseCustomerResponse(resp)
}

// GetCustomer retrieves a single customer by code.
func (c *Client) GetCustomer(ctx context.Context, code string) (*Customer, error) {
	resp, err := c.getOne(ctx, registerCustomer, code)
	if err != nil {
		return nil, err
	}

	customers, _, err := parseCustomerResponse(resp)
	if err != nil {
		return nil, err
	}
	if len(customers) == 0 {
		return nil, &APIError{StatusCode: 404, Message: "customer not found"}
	}
	return &customers[0], nil
}

// CreateCustomer creates a new contact/customer.
func (c *Client) CreateCustomer(ctx context.Context, fields map[string]string) (*Customer, error) {
	resp, err := c.post(ctx, registerCustomer, fields)
	if err != nil {
		return nil, err
	}

	customers, _, err := parseCustomerResponse(resp)
	if err != nil {
		return nil, err
	}
	if len(customers) == 0 {
		return nil, fmt.Errorf("excellentbooks: no customer returned after create")
	}
	return &customers[0], nil
}

// UpdateCustomer updates an existing customer by code.
func (c *Client) UpdateCustomer(ctx context.Context, code string, fields map[string]string) error {
	_, err := c.patch(ctx, registerCustomer, code, fields)
	return err
}

func parseCustomerResponse(resp *Response) ([]Customer, string, error) {
	var envelope struct {
		ResponseMeta
		CUVc []Customer `json:"CUVc"`
	}
	if err := json.Unmarshal(resp.Data, &envelope); err != nil {
		return nil, "", fmt.Errorf("excellentbooks: parse customers: %w", err)
	}
	return envelope.CUVc, envelope.Sequence, nil
}
