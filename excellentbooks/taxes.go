package excellentbooks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

const registerVATCode = "VATCodeBlock"

// ListVATCodes retrieves all VAT/tax codes from the VATCodeBlock register.
func (c *Client) ListVATCodes(ctx context.Context, params ListParams) ([]VATCode, string, error) {
	resp, err := c.get(ctx, registerVATCode, params)
	if err != nil {
		return nil, "", err
	}
	return parseVATCodeResponse(resp)
}

func parseVATCodeResponse(resp *Response) ([]VATCode, string, error) {
	// Standard Books wraps VAT codes in a {rows: [...]} object rather than
	// returning a direct array (unlike most other registers).
	var envelope struct {
		ResponseMeta
		VATCodeBlock struct {
			Rows []VATCode `json:"rows"`
		} `json:"VATCodeBlock"`
	}
	if err := json.Unmarshal(resp.Data, &envelope); err != nil {
		slog.Error("excellentbooks: parse vat codes failed", "raw_data", string(resp.Data))
		return nil, "", fmt.Errorf("excellentbooks: parse vat codes: %w", err)
	}
	if len(envelope.VATCodeBlock.Rows) == 0 {
		slog.Warn("excellentbooks: VATCodeBlock returned 0 items", "raw_data", string(resp.Data))
	}
	return envelope.VATCodeBlock.Rows, envelope.Sequence, nil
}
