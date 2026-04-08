package directo

import (
	"context"
	"encoding/xml"
	"net/url"
)

// ItemXML represents an item/article for XML Direct write operations.
type ItemXML struct {
	XMLName     xml.Name `xml:"item"`
	Code        string   `xml:"code,attr"`
	Name        string   `xml:"name,attr"`
	Description string   `xml:"description,attr,omitempty"`
	Type        string   `xml:"type,attr,omitempty"` // 0=service, 1=stock item, 2=rental
	Unit        string   `xml:"unit,attr,omitempty"`
	Price       string   `xml:"price,attr,omitempty"`
	TaxCode     string   `xml:"tax,attr,omitempty"`
	Class       string   `xml:"class,attr,omitempty"`
	Barcode     string   `xml:"barcode,attr,omitempty"`
	SalesAcc    string   `xml:"salesacc,attr,omitempty"`
	PurchaseAcc string   `xml:"purchaseacc,attr,omitempty"`
}

// itemsXMLWrapper wraps item(s) for XML Direct submission.
type itemsXMLWrapper struct {
	XMLName xml.Name  `xml:"items"`
	Items   []ItemXML `xml:"item"`
}

// ListItems retrieves items via REST API.
func (c *Client) ListItems(ctx context.Context, params ItemListParams) ([]ItemREST, error) {
	qp := url.Values{}
	if params.Code != "" {
		qp.Set("code", params.Code)
	}
	if params.Class != "" {
		qp.Set("class", params.Class)
	}
	if params.Status != "" {
		qp.Set("status", params.Status)
	}
	if params.TSFrom != "" {
		qp.Set("ts", ">"+params.TSFrom)
	}

	var result []ItemREST
	err := c.rest.get(ctx, "items", qp, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetItem retrieves a single item by code via REST API.
func (c *Client) GetItem(ctx context.Context, code string) (*ItemREST, error) {
	params := url.Values{"code": {code}}
	var result []ItemREST
	err := c.rest.get(ctx, "items", params, &result)
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, &APIError{StatusCode: 404, Message: "item not found", Source: "rest"}
	}
	return &result[0], nil
}

// CreateItem creates an item via XML Direct.
func (c *Client) CreateItem(ctx context.Context, item ItemXML) (*XMLResults, error) {
	wrapper := itemsXMLWrapper{
		Items: []ItemXML{item},
	}

	xmlData, err := xml.Marshal(wrapper)
	if err != nil {
		return nil, err
	}

	return c.xml.xmlPut(ctx, "item", string(xmlData), nil)
}

// UpdateItem updates an item via XML Direct.
// Directo uses upsert semantics — same endpoint for create and update.
func (c *Client) UpdateItem(ctx context.Context, item ItemXML) (*XMLResults, error) {
	return c.CreateItem(ctx, item)
}
