package excellentbooks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

const registerPaymentTerm = "PDVc"

// ListPaymentTerms retrieves all payment-term register entries (PDVc).
// Each term has a Code (e.g. "K", "P14"), a Comment (human label), and a
// PDType (1=Regular net-days term, 2=Cash/immediate, 3=Credit, 4=Next month).
func (c *Client) ListPaymentTerms(ctx context.Context, params ListParams) ([]PaymentTerm, string, error) {
	resp, err := c.get(ctx, registerPaymentTerm, params)
	if err != nil {
		return nil, "", err
	}
	var envelope struct {
		ResponseMeta
		PDVc []PaymentTerm `json:"PDVc"`
	}
	if err := json.Unmarshal(resp.Data, &envelope); err != nil {
		slog.Error("excellentbooks: parse payment terms failed", "raw_data", string(resp.Data))
		return nil, "", fmt.Errorf("excellentbooks: parse payment terms: %w", err)
	}
	if len(envelope.PDVc) == 0 {
		slog.Warn("excellentbooks: PDVc returned 0 items", "raw_data", string(resp.Data))
	}
	return envelope.PDVc, envelope.Sequence, nil
}
