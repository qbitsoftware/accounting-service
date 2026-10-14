package excellentbooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
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

// PostReceipt creates a receipt like CreateReceipt but never fails over the
// answer once Excellent Books has accepted the write: a 2xx without an error
// payload is success, and the saved record is returned when the answer carries
// one, nil when it does not (no data, no IPVc, an empty list, or a body that
// does not decode). The receipt exists either way; a caller that needs to find
// it again does so by what it wrote into it (the header Comment).
func (c *Client) PostReceipt(ctx context.Context, fields map[string]string) (*Receipt, error) {
	resp, err := c.postLenient(ctx, registerReceipt, fields)
	if err != nil {
		return nil, err
	}
	if noData(resp.Data) {
		return nil, nil
	}
	receipts, _, perr := parseReceiptResponse(resp)
	if perr != nil || len(receipts) == 0 {
		if perr != nil {
			slog.Warn("excellentbooks: receipt saved but the answer did not decode; SerNr unknown", "error", perr)
		}
		return nil, nil
	}
	return &receipts[0], nil
}

// GetReceiptRecords reads one receipt by SerNr and returns every record the
// answer carries — normally one. An empty slice means Excellent Books answered
// 2xx with no record at all (no data, no IPVc, or an empty list); a 404 comes
// back as an *APIError. Choosing the record, and deciding what an answer about
// some other SerNr means, is the caller's.
func (c *Client) GetReceiptRecords(ctx context.Context, serNr string) ([]Receipt, error) {
	resp, err := c.getOne(ctx, registerReceipt, serNr)
	if err != nil {
		return nil, err
	}
	if noData(resp.Data) {
		return nil, nil
	}
	receipts, _, err := parseReceiptResponse(resp)
	if err != nil {
		return nil, err
	}
	return receipts, nil
}

// DeleteRaw deletes one record by id and returns the raw `data` payload. For
// diagnostics only (cmd/eb-onewriter-probe, sandbox item E7): no adapter method
// deletes anything in Excellent Books — a confirmed receipt cannot be deleted
// there at all, and is undone by its negative copy instead.
func (c *Client) DeleteRaw(ctx context.Context, register, id string) (json.RawMessage, error) {
	resp, err := c.deleteOne(ctx, register, id)
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// DeletedReceipt is one record of a deletes_after answer.
type DeletedReceipt struct {
	SerNr    string
	Sequence string
}

// DeletedReceipts is a deletes_after answer, decoded without assuming its
// shape (undocumented; sandbox item E2).
type DeletedReceipts struct {
	// Recognised is true only when the answer was a list (or a single object)
	// of records under the IPVc key, each carrying a SerNr — an empty list
	// included. Records is meaningless otherwise.
	Recognised bool
	Records    []DeletedReceipt
	// Count is how many elements the IPVc list held, recognised or not.
	Count int
	// Sequence is the answer's data@sequence.
	Sequence string
	// Raw is the undecoded data payload, for diagnostics.
	Raw json.RawMessage
}

// ListDeletedReceipts asks IPVc for the receipts deleted after a sequence
// (deletes_after) and decodes the answer defensively. A register that refuses
// the parameter answers an error like any other request; one that ignores it
// answers live receipts, which decode here exactly like deleted ones — telling
// the two apart is the caller's (Records carry their @sequence for that).
func (c *Client) ListDeletedReceipts(ctx context.Context, afterSequence string, limit int) (*DeletedReceipts, error) {
	resp, err := c.get(ctx, registerReceipt, ListParams{DeletesAfter: afterSequence, Limit: limit})
	if err != nil {
		return nil, err
	}
	return decodeDeletedReceipts(resp.Data), nil
}

func decodeDeletedReceipts(data json.RawMessage) *DeletedReceipts {
	out := &DeletedReceipts{Raw: data}
	var env map[string]json.RawMessage
	if json.Unmarshal(data, &env) != nil {
		return out
	}
	if raw, ok := env["@sequence"]; ok {
		var seq FlexString
		if json.Unmarshal(raw, &seq) == nil {
			out.Sequence = string(seq)
		}
	}
	raw, ok := env[registerReceipt]
	if !ok {
		return out
	}
	var elems []json.RawMessage
	if json.Unmarshal(raw, &elems) != nil {
		var one map[string]json.RawMessage
		if json.Unmarshal(raw, &one) != nil {
			return out
		}
		elems = []json.RawMessage{raw}
	}
	out.Count = len(elems)
	recs := make([]DeletedReceipt, 0, len(elems))
	for _, e := range elems {
		var m map[string]json.RawMessage
		if json.Unmarshal(e, &m) != nil {
			return out
		}
		var serNr, seq FlexString
		if json.Unmarshal(m["SerNr"], &serNr) != nil || serNr == "" || strings.ContainsAny(string(serNr), "{[") {
			return out
		}
		if rawSeq, ok := m["@sequence"]; ok {
			if json.Unmarshal(rawSeq, &seq) != nil {
				seq = ""
			}
		}
		recs = append(recs, DeletedReceipt{SerNr: string(serNr), Sequence: string(seq)})
	}
	out.Records = recs
	out.Recognised = true
	return out
}

// noData reports whether a response's data payload is absent or null.
func noData(data json.RawMessage) bool {
	t := bytes.TrimSpace(data)
	return len(t) == 0 || bytes.Equal(t, []byte("null")) || bytes.Equal(t, []byte("{}"))
}
