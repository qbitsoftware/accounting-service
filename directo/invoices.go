package directo

import (
	"context"
	"encoding/xml"
	"net/url"
)

// invoiceRowsWrapper wraps invoice line rows per xml_IN_arved.xsd (<rows><row .../></rows>).
type invoiceRowsWrapper struct {
	XMLName xml.Name         `xml:"rows"`
	Rows    []InvoiceLineXML `xml:"row"`
}

// InvoiceXML represents a sales invoice for XML Direct write operations.
// Field names match the xml_IN_arved.xsd schema exactly.
//
// The Email/CustomerRegNo/CustomerType/Address fields let you stamp
// customer details directly on the invoice — useful when the customer
// write component is not licensed and the referenced customercode either
// doesn't exist or lacks an email/reg.no on file. Directo will use these
// inline values to satisfy its "missing customer email or reg.no" check
// (validation error type="13").
type InvoiceXML struct {
	XMLName      xml.Name           `xml:"invoice"`
	Number       string             `xml:"number,attr,omitempty"`
	CustomerCode string             `xml:"customercode,attr"`           // XSD: customercode (was customer)
	CustomerName string             `xml:"customername,attr,omitempty"`
	Date         string             `xml:"date,attr"`
	Deadline     string             `xml:"duedate,attr,omitempty"`      // XSD: duedate (was deadline)
	Currency     string             `xml:"currency,attr,omitempty"`
	Comment      string             `xml:"comment,attr,omitempty"`
	VATZone      string             `xml:"vatzone,attr,omitempty"`      // 0=local, 1=EU, 2=export
	Language     string             `xml:"language,attr,omitempty"`
	PaymentTerm  string             `xml:"paymentterm,attr,omitempty"`  // XSD: paymentterm
	PaymentTotal string             `xml:"paymenttotal,attr,omitempty"` // positive = auto-create receipt
	Confirm      string             `xml:"confirm,attr,omitempty"`      // XSD: confirm (was confirmed), "1" = confirm

	// Inline customer details — see struct doc comment.
	Email        string `xml:"email,attr,omitempty"`
	Phone        string `xml:"phone,attr,omitempty"`
	Address1     string `xml:"address1,attr,omitempty"`
	Address2     string `xml:"address2,attr,omitempty"`
	Address3     string `xml:"address3,attr,omitempty"`
	VATRegNo     string `xml:"vatregno,attr,omitempty"`      // VAT registration number
	CustomerRegNo  string `xml:"customer_regno,attr,omitempty"`  // company registration number
	CustomerType   string `xml:"customer_type,attr,omitempty"`   // 0=company, 1=private, 2=government

	Rows invoiceRowsWrapper
}

// InvoiceLineXML represents an invoice row per xml_IN_arved.xsd.
type InvoiceLineXML struct {
	ItemCode    string `xml:"item,attr,omitempty"`        // XSD: item (was code)
	Description string `xml:"description,attr,omitempty"`
	Quantity    string `xml:"quantity,attr"`
	Price       string `xml:"price,attr"`
	VatCode     string `xml:"vatcode,attr,omitempty"`     // XSD: vatcode (was tax)
	AccountCode string `xml:"account,attr,omitempty"`
	Unit        string `xml:"unit,attr,omitempty"`
	Object      string `xml:"object,attr,omitempty"`
	Project     string `xml:"project,attr,omitempty"`
}

// NewInvoiceRows is a convenience constructor for the rows wrapper.
func NewInvoiceRows(rows []InvoiceLineXML) invoiceRowsWrapper {
	return invoiceRowsWrapper{Rows: rows}
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
// Uses what=invoice per the XML Direct components table — note that
// "arved" is only the Estonian UI label / XSD filename, not the API value.
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
	xmlData := `<invoices><invoice number="` + number + `" delete="1"><rows></rows></invoice></invoices>`
	return c.xml.xmlPut(ctx, "invoice", xmlData, nil)
}
