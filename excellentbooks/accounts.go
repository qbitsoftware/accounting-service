package excellentbooks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

const registerGLAccount = "AccVc"

// ListGLAccounts retrieves all chart-of-accounts entries from the AccVc register.
// (Method name kept for API stability; the underlying register is Standard Books'
// AccVc, not the non-existent GLAccounts that earlier doc scrapes suggested.)
func (c *Client) ListGLAccounts(ctx context.Context, params ListParams) ([]GLAccount, string, error) {
	resp, err := c.get(ctx, registerGLAccount, params)
	if err != nil {
		return nil, "", err
	}
	return parseGLAccountResponse(resp)
}

func parseGLAccountResponse(resp *Response) ([]GLAccount, string, error) {
	var envelope struct {
		ResponseMeta
		AccVc []GLAccount `json:"AccVc"`
	}
	if err := json.Unmarshal(resp.Data, &envelope); err != nil {
		slog.Error("excellentbooks: parse gl accounts failed", "raw_data", string(resp.Data))
		return nil, "", fmt.Errorf("excellentbooks: parse gl accounts: %w", err)
	}

	// Phase 3 diagnostic: parse the response into a generic map so we can log the
	// exact JSON keys present on the first row. With that we can fix the field
	// tags without further round-trips. Remove once stable.
	var probe struct {
		AccVc []map[string]interface{} `json:"AccVc"`
	}
	if json.Unmarshal(resp.Data, &probe) == nil && len(probe.AccVc) > 0 {
		first := probe.AccVc[0]
		keys := make([]string, 0, len(first))
		for k := range first {
			keys = append(keys, k)
		}
		slog.Info("excellentbooks: AccVc first-row field probe (Phase 3 diagnostic)",
			"keys", keys, "first_row", first)
	}

	if len(envelope.AccVc) == 0 {
		slog.Warn("excellentbooks: AccVc returned 0 items", "raw_data", string(resp.Data))
	}
	return envelope.AccVc, envelope.Sequence, nil
}
