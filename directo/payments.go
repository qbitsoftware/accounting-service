package directo

import (
	"context"
	"encoding/xml"
	"net/url"
)

// ReceiptXML represents a receipt/payment for XML Direct write operations.
type ReceiptXML struct {
	XMLName      xml.Name         `xml:"receipt"`
	Number       string           `xml:"number,attr,omitempty"`
	CustomerCode string           `xml:"customer,attr"`
	Date         string           `xml:"date,attr"`
	Currency     string           `xml:"currency,attr,omitempty"`
	BankAccount  string           `xml:"bankaccount,attr,omitempty"`
	Lines        []ReceiptLineXML `xml:"line"`
}

// ReceiptLineXML represents a receipt line linking a payment to an invoice.
type ReceiptLineXML struct {
	XMLName   xml.Name `xml:"line"`
	InvoiceNo string   `xml:"invoiceno,attr"`
	Amount    string   `xml:"amount,attr"`
}

// receiptsXMLWrapper wraps receipt(s) for XML Direct submission.
type receiptsXMLWrapper struct {
	XMLName  xml.Name     `xml:"receipts"`
	Receipts []ReceiptXML `xml:"receipt"`
}

// ListPayments retrieves receipts/payments via REST API.
func (c *Client) ListPayments(ctx context.Context, params PaymentListParams) ([]ReceiptREST, error) {
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

	var result []ReceiptREST
	err := c.rest.get(ctx, "receipts", qp, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CreatePayment creates a receipt/payment via XML Direct.
func (c *Client) CreatePayment(ctx context.Context, receipt ReceiptXML) (*XMLResults, error) {
	wrapper := receiptsXMLWrapper{
		Receipts: []ReceiptXML{receipt},
	}

	xmlData, err := xml.Marshal(wrapper)
	if err != nil {
		return nil, err
	}

	return c.xml.xmlPut(ctx, "receipt", string(xmlData), nil)
}

// DeletePayment deletes a receipt by number via XML Direct.
func (c *Client) DeletePayment(ctx context.Context, number string) (*XMLResults, error) {
	xmlData := `<receipts><receipt number="` + number + `" delete="1" /></receipts>`
	return c.xml.xmlPut(ctx, "receipt", xmlData, nil)
}
