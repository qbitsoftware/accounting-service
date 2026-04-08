package directo

import (
	"context"
	"encoding/xml"
	"fmt"
)

// TaxXML represents a tax entry from XML Direct GET response.
type TaxXML struct {
	XMLName xml.Name `xml:"tax"`
	Code    string   `xml:"code,attr"`
	Name    string   `xml:"name,attr"`
	Pct     string   `xml:"pct,attr"`
}

// taxesXMLResponse wraps the XML response for tax list.
type taxesXMLResponse struct {
	XMLName xml.Name `xml:"taxes"`
	Taxes   []TaxXML `xml:"tax"`
}

// ListTaxes retrieves tax/VAT codes.
// Tries REST API first, falls back to XML Direct if not available.
func (c *Client) ListTaxes(ctx context.Context) ([]TaxXML, error) {
	// Try XML Direct GET for tax data
	body, err := c.xml.xmlGet(ctx, "tax", nil)
	if err != nil {
		return nil, err
	}

	var resp taxesXMLResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("directo: unmarshal taxes: %w (body: %s)", err, string(body))
	}

	return resp.Taxes, nil
}
