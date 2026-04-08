package directo

import (
	"context"
	"encoding/xml"
	"net/url"
)

// InvoiceXML represents a sales invoice for XML Direct write operations.
type InvoiceXML struct {
	XMLName      xml.Name         `xml:"invoice"`
	Number       string           `xml:"number,attr,omitempty"`
	CustomerCode string           `xml:"customer,attr"`
	CustomerName string           `xml:"customername,attr,omitempty"`
	Date         string           `xml:"date,attr"`
	Deadline     string           `xml:"deadline,attr"`
	Currency     string           `xml:"currency,attr,omitempty"`
	RefNo        string           `xml:"refno,attr,omitempty"`
	Comment      string           `xml:"comment,attr,omitempty"`
	FootComment  string           `xml:"footcomment,attr,omitempty"`
	VATZone      string           `xml:"vatzone,attr,omitempty"` // 0=local, 1=EU, 2=export
	Language     string           `xml:"language,attr,omitempty"`
	PaymentDays  string           `xml:"paymentdays,attr,omitempty"`
	PaymentTotal string           `xml:"paymenttotal,attr,omitempty"` // If positive, auto-creates receipt
	Confirmed    string           `xml:"confirmed,attr,omitempty"`    // 1 = auto-confirm
	Lines        []InvoiceLineXML `xml:"line"`
}

// InvoiceLineXML represents an invoice line for XML Direct.
type InvoiceLineXML struct {
	XMLName     xml.Name `xml:"line"`
	ItemCode    string   `xml:"code,attr"`
	Description string   `xml:"description,attr,omitempty"`
	Quantity    string   `xml:"quantity,attr"`
	Price       string   `xml:"price,attr"`
	TaxCode     string   `xml:"tax,attr,omitempty"`
	AccountCode string   `xml:"account,attr,omitempty"`
	Unit        string   `xml:"unit,attr,omitempty"`
	Object      string   `xml:"object,attr,omitempty"` // Dimension/cost center
	Project     string   `xml:"project,attr,omitempty"`
}

// invoicesXMLWrapper wraps invoice(s) for XML Direct submission.
type invoicesXMLWrapper struct {
	XMLName  xml.Name     `xml:"invoices"`
	Invoices []InvoiceXML `xml:"invoice"`
}

// ListInvoices retrieves invoices via REST API.
func (c *Client) ListInvoices(ctx context.Context, params InvoiceListParams) ([]InvoiceREST, error) {
	qp := url.Values{}
	if params.DateFrom != "" {
		qp.Set("date", ">"+params.DateFrom)
	}
	if params.DateTo != "" {
		qp.Add("date", "<"+params.DateTo)
	}
	if params.TSFrom != "" {
		qp.Set("ts", ">"+params.TSFrom)
	}
	if params.Status != "" {
		qp.Set("status", params.Status)
	}

	var result []InvoiceREST
	err := c.rest.get(ctx, "invoices", qp, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetInvoice retrieves a single invoice by number via REST API.
func (c *Client) GetInvoice(ctx context.Context, number string) (*InvoiceREST, error) {
	params := url.Values{"number": {number}}
	var result []InvoiceREST
	err := c.rest.get(ctx, "invoices", params, &result)
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, &APIError{StatusCode: 404, Message: "invoice not found", Source: "rest"}
	}
	return &result[0], nil
}

// CreateInvoice creates an invoice via XML Direct.
func (c *Client) CreateInvoice(ctx context.Context, inv InvoiceXML, extraParams url.Values) (*XMLResults, error) {
	wrapper := invoicesXMLWrapper{
		Invoices: []InvoiceXML{inv},
	}

	xmlData, err := xml.Marshal(wrapper)
	if err != nil {
		return nil, err
	}

	return c.xml.xmlPut(ctx, "invoice", string(xmlData), extraParams)
}

// DeleteInvoice deletes an invoice by number via XML Direct.
func (c *Client) DeleteInvoice(ctx context.Context, number string) (*XMLResults, error) {
	// Directo uses a special delete operation via XML
	xmlData := `<invoices><invoice number="` + number + `" delete="1" /></invoices>`
	return c.xml.xmlPut(ctx, "invoice", xmlData, nil)
}
