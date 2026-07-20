package simplbooks

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ListIncomings returns customer receipts (money in) matching the params.
func (c *Client) ListIncomings(ctx context.Context, params ListParams) ([]Incoming, error) {
	resp, err := c.do(ctx, "GET", "incomings/list", params.body())
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Incoming Incoming `json:"Incoming"`
	}
	if err := resp.decodeData(&rows); err != nil {
		return nil, fmt.Errorf("simplbooks: parse incomings: %w", err)
	}
	out := make([]Incoming, len(rows))
	for i := range rows {
		out[i] = rows[i].Incoming
	}
	return out, nil
}

// ListAllIncomings pages through incomings/list until exhausted.
func (c *Client) ListAllIncomings(ctx context.Context, filter map[string]any, perPage int) ([]Incoming, error) {
	if perPage <= 0 {
		perPage = 100
	}
	var all []Incoming
	for page := 1; ; page++ {
		batch, err := c.ListIncomings(ctx, ListParams{Page: page, PerPage: perPage, Filter: filter})
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < perPage {
			break
		}
	}
	return all, nil
}

// CreateIncoming records a customer receipt. When invoiceID is non-zero the
// receipt is bound to that invoice (settling it if the sum covers the balance).
func (c *Client) CreateIncoming(ctx context.Context, fields map[string]any, invoiceID int) (int, error) {
	body := map[string]any{"Incoming": fields}
	if invoiceID != 0 {
		body["invoice_id"] = invoiceID
	}
	resp, err := c.do(ctx, "POST", "incomings/create", body)
	if err != nil {
		return 0, err
	}
	return resp.insertedID()
}

// BindIncomingToInvoice ties an existing receipt to an invoice, marking the
// invoice paid when the receipt sum is large enough.
func (c *Client) BindIncomingToInvoice(ctx context.Context, incomingID, invoiceID int, fullCover bool) error {
	// Read the allocation state BEFORE binding: `bound` alone can't distinguish
	// "this call consumed the last of it" from "it was already spent".
	boundBefore := false
	if before, err := c.GetIncoming(ctx, strconv.Itoa(incomingID)); err == nil {
		boundBefore = before.Bound
	}

	resp, err := c.do(ctx, "POST", "incomings/invoice_paid", map[string]any{
		"incoming_id": incomingID,
		"invoice_id":  invoiceID,
		"full_cover":  fullCover,
	})
	if err != nil {
		return err
	}
	// An exhausted receipt is NOT an API error: SimplBooks answers HTTP 200 with
	// data:true and only the human-readable message says nothing happened
	// ("Laekumine on juba täies mahus arvetega seotud" — already fully bound),
	// leaving the invoice untouched. Verified live. Treating that as success
	// would let a caller record an allocation that never happened.
	//
	// That message is localised per API user and is unversioned server copy, so
	// it cannot be the only signal: confirm against the receipt's own `bound`
	// flag, which is structured data. Order matters — the string check is the
	// cheap path, the read-back is the authority.
	if containsAny(resp.Response, alreadyAllocatedMessages) {
		return fmt.Errorf("%w: receipt %d is already fully allocated to invoices", ErrAlreadyAllocated, incomingID)
	}
	inc, getErr := c.GetIncoming(ctx, strconv.Itoa(incomingID))
	if getErr != nil {
		// Can't verify — report success rather than inventing a failure for a
		// bind the server accepted. The caller reconciles against the invoice.
		return nil
	}
	if inc.Bound && !boundBefore {
		return nil // newly exhausted by THIS bind — the allocation landed
	}
	if inc.Bound && boundBefore {
		return fmt.Errorf("%w: receipt %d was already fully allocated before this bind", ErrAlreadyAllocated, incomingID)
	}
	return nil
}

// alreadyAllocatedMessages are the known localisations of SimplBooks' "receipt
// already fully bound to invoices" response. Matched case-insensitively on a
// distinctive fragment so minor copy edits don't break detection.
var alreadyAllocatedMessages = []string{
	"juba täies mahus", // et
	"already",          // en (…"is already fully bound"…)
	"полностью",        // ru
}

func containsAny(haystack string, needles []string) bool {
	h := strings.ToLower(haystack)
	for _, n := range needles {
		if strings.Contains(h, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

// ErrAlreadyAllocated reports that a receipt had no unallocated amount left, so
// binding it to a further invoice was a no-op.
var ErrAlreadyAllocated = errors.New("simplbooks: receipt already fully allocated")

// GetIncoming fetches a single receipt, including its `bound` allocation flag.
func (c *Client) GetIncoming(ctx context.Context, id string) (*Incoming, error) {
	resp, err := c.do(ctx, "GET", "incomings/get/"+id, nil)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Incoming Incoming `json:"Incoming"`
	}
	if err := resp.decodeData(&wrapper); err != nil {
		return nil, fmt.Errorf("simplbooks: parse incoming: %w", err)
	}
	return &wrapper.Incoming, nil
}

// DeleteIncoming deletes a customer receipt.
func (c *Client) DeleteIncoming(ctx context.Context, id string) error {
	_, err := c.do(ctx, "POST", "incomings/delete/"+id, nil)
	return err
}
