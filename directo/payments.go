package directo

import (
	"context"
	"encoding/xml"
	"net/url"
)

// receiptRowsWrapper wraps receipt rows per xml_IN_laekumised.xsd
// (<rows><row .../></rows>).
type receiptRowsWrapper struct {
	XMLName xml.Name        `xml:"rows"`
	Rows    []ReceiptRowXML `xml:"row"`
}

// ReceiptXML represents a receipt/payment ("laekumine") for XML Direct
// write operations. Field names match the xml_IN_laekumised.xsd schema
// exactly — the schema is strict and silently IGNORES anything it doesn't
// recognise, so a wrong attribute name doesn't error, it just produces a
// hollow document. Hard-won specifics:
//   - Number is REQUIRED ("Missing document identificator", result type 12,
//     without it) and must stay the same across re-sends of the document.
//   - PaymentMode is the Tasumisviis register code. It determines the debit
//     account; confirming fails with "Tasumisviisi ei leitud / Deebet on
//     vale või puudu" when missing or unknown.
//   - The customer and the settled invoice live on the ROW, not the header.
type ReceiptXML struct {
	XMLName     xml.Name `xml:"receipt"`
	Number      string   `xml:"number,attr"`
	Date        string   `xml:"date,attr,omitempty"`
	Description string   `xml:"description,attr,omitempty"`
	PaymentMode string   `xml:"paymentmode,attr,omitempty"` // XSD: tasumisviis
	// Confirm "1" books the receipt immediately (kinnitatud). An
	// unconfirmed receipt does NOT settle its invoice — someone has to
	// press Kinnita in Directo's UI — so callers that want the invoice
	// to flip to paid must set this.
	Confirm string `xml:"confirm,attr,omitempty"`
	Rows    receiptRowsWrapper
}

// NewReceiptRows is a convenience constructor for the rows wrapper.
func NewReceiptRows(rows []ReceiptRowXML) receiptRowsWrapper {
	return receiptRowsWrapper{Rows: rows}
}

// ReceiptRowXML is one receipt row per xml_IN_laekumised.xsd — allocates a
// received amount against one sales invoice.
type ReceiptRowXML struct {
	InvoiceNo    string `xml:"invoice,attr,omitempty"`      // XSD: invoice (arvenumber)
	CustomerCode string `xml:"customer,attr,omitempty"`     // XSD: customer (klient_kood)
	Payment      string `xml:"payment,attr,omitempty"`      // XSD: payment (tasuti)
	Received     string `xml:"received,attr,omitempty"`     // XSD: received (summa_p)
	BankCurrency string `xml:"bankcurrency,attr,omitempty"` // XSD: bankcurrency (valuuta_p)
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
