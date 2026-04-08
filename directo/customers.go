package directo

import (
	"context"
	"encoding/xml"
	"net/url"
)

// CustomerXML represents a customer for XML Direct write operations.
type CustomerXML struct {
	XMLName     xml.Name `xml:"customer"`
	Code        string   `xml:"code,attr"`
	Name        string   `xml:"name,attr"`
	RegNo       string   `xml:"regnr,attr,omitempty"`
	VATNo       string   `xml:"vatregnr,attr,omitempty"`
	Email       string   `xml:"email,attr,omitempty"`
	Phone       string   `xml:"phone,attr,omitempty"`
	Address     string   `xml:"address,attr,omitempty"`
	City        string   `xml:"city,attr,omitempty"`
	PostalCode  string   `xml:"postalcode,attr,omitempty"`
	Country     string   `xml:"country,attr,omitempty"`
	County      string   `xml:"county,attr,omitempty"`
	Currency    string   `xml:"currency,attr,omitempty"`
	Contact     string   `xml:"contact,attr,omitempty"`
	Type        string   `xml:"type,attr,omitempty"`        // 0=company, 1=private, 2=government
	PaymentDays string   `xml:"paymentdays,attr,omitempty"` // Payment deadline in days
	HomePage    string   `xml:"homepage,attr,omitempty"`
}

// customersXMLWrapper wraps customer(s) for XML Direct submission.
type customersXMLWrapper struct {
	XMLName   xml.Name      `xml:"customers"`
	Customers []CustomerXML `xml:"customer"`
}

// ListCustomers retrieves all customers via REST API.
func (c *Client) ListCustomers(ctx context.Context) ([]CustomerREST, error) {
	var result []CustomerREST
	err := c.rest.get(ctx, "customers", nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ListCustomersSince retrieves customers changed since the given timestamp via REST API.
func (c *Client) ListCustomersSince(ctx context.Context, ts string) ([]CustomerREST, error) {
	params := url.Values{}
	if ts != "" {
		params.Set("ts", ">"+ts)
	}
	var result []CustomerREST
	err := c.rest.get(ctx, "customers", params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetCustomerByCode retrieves a single customer by code via REST API.
func (c *Client) GetCustomerByCode(ctx context.Context, code string) (*CustomerREST, error) {
	params := url.Values{"code": {code}}
	var result []CustomerREST
	err := c.rest.get(ctx, "customers", params, &result)
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, &APIError{StatusCode: 404, Message: "customer not found", Source: "rest"}
	}
	return &result[0], nil
}

// GetCustomerByEmail retrieves customers filtered by email via REST API.
func (c *Client) GetCustomerByEmail(ctx context.Context, email string) ([]CustomerREST, error) {
	params := url.Values{"email": {email}}
	var result []CustomerREST
	err := c.rest.get(ctx, "customers", params, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CreateCustomer creates a customer via XML Direct.
func (c *Client) CreateCustomer(ctx context.Context, cust CustomerXML) (*XMLResults, error) {
	wrapper := customersXMLWrapper{
		Customers: []CustomerXML{cust},
	}

	xmlData, err := xml.Marshal(wrapper)
	if err != nil {
		return nil, err
	}

	return c.xml.xmlPut(ctx, "customer", string(xmlData), nil)
}

// UpdateCustomer updates a customer via XML Direct.
// Directo uses upsert semantics — same endpoint for create and update.
func (c *Client) UpdateCustomer(ctx context.Context, cust CustomerXML) (*XMLResults, error) {
	return c.CreateCustomer(ctx, cust)
}
