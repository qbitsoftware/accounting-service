package excellentbooks

import (
	"context"
	"encoding/json"
	"fmt"
)

const registerPurchase = "VIVc"

// ListPurchases retrieves purchase invoices.
func (c *Client) ListPurchases(ctx context.Context, params ListParams) ([]PurchaseInvoice, string, error) {
	resp, err := c.get(ctx, registerPurchase, params)
	if err != nil {
		return nil, "", err
	}
	return parsePurchaseResponse(resp)
}

func parsePurchaseResponse(resp *Response) ([]PurchaseInvoice, string, error) {
	var envelope struct {
		ResponseMeta
		VIVc []PurchaseInvoice `json:"VIVc"`
	}
	if err := json.Unmarshal(resp.Data, &envelope); err != nil {
		return nil, "", fmt.Errorf("excellentbooks: parse purchases: %w", err)
	}
	return envelope.VIVc, envelope.Sequence, nil
}
