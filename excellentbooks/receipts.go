package excellentbooks

import (
	"context"
	"encoding/json"
	"fmt"
)

const registerReceipt = "IPVc"

// ListReceipts retrieves incoming payments/receipts.
func (c *Client) ListReceipts(ctx context.Context, params ListParams) ([]Receipt, string, error) {
	resp, err := c.get(ctx, registerReceipt, params)
	if err != nil {
		return nil, "", err
	}
	return parseReceiptResponse(resp)
}

// GetReceipt retrieves a single receipt by serial number.
func (c *Client) GetReceipt(ctx context.Context, serNr string) (*Receipt, error) {
	resp, err := c.getOne(ctx, registerReceipt, serNr)
	if err != nil {
		return nil, err
	}

	receipts, _, err := parseReceiptResponse(resp)
	if err != nil {
		return nil, err
	}
	if len(receipts) == 0 {
		return nil, &APIError{StatusCode: 404, Message: "receipt not found"}
	}
	return &receipts[0], nil
}

// CreateReceipt creates a new incoming payment/receipt.
func (c *Client) CreateReceipt(ctx context.Context, fields map[string]string) (*Receipt, error) {
	resp, err := c.post(ctx, registerReceipt, fields)
	if err != nil {
		return nil, err
	}

	receipts, _, err := parseReceiptResponse(resp)
	if err != nil {
		return nil, err
	}
	if len(receipts) == 0 {
		return nil, fmt.Errorf("excellentbooks: no receipt returned after create")
	}
	return &receipts[0], nil
}

// parseReceiptResponse extracts receipts and sequence from the response.
func parseReceiptResponse(resp *Response) ([]Receipt, string, error) {
	var envelope struct {
		ResponseMeta
		IPVc []Receipt `json:"IPVc"`
	}
	if err := json.Unmarshal(resp.Data, &envelope); err != nil {
		var singleEnvelope struct {
			ResponseMeta
			IPVc json.RawMessage `json:"IPVc"`
		}
		if err2 := json.Unmarshal(resp.Data, &singleEnvelope); err2 != nil {
			return nil, "", fmt.Errorf("excellentbooks: parse receipts: %w", err)
		}
		var receipts []Receipt
		if err2 := json.Unmarshal(singleEnvelope.IPVc, &receipts); err2 != nil {
			var r Receipt
			if err3 := json.Unmarshal(singleEnvelope.IPVc, &r); err3 != nil {
				return nil, "", fmt.Errorf("excellentbooks: parse receipts: %w", err)
			}
			receipts = []Receipt{r}
		}
		return receipts, singleEnvelope.Sequence, nil
	}
	return envelope.IPVc, envelope.Sequence, nil
}
