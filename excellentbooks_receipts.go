package accounting

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/qbitsoftware/accounting-service/excellentbooks"
	"github.com/shopspring/decimal"
)

// Excellent Books receipt-level operations (eb-receipts). These are new code
// paths beside CreatePayment / ListPayments / ApplyPrepayment, which stay
// exactly as they were (excellentbooks_golden_test.go): a club that is not
// moved onto receipt-level behaviour never reaches anything in this file.
//
// What Excellent Books is, for a receipt (design: the backend's
// docs/EXCELLENTBOOKS_RECEIPT_OWNERSHIP.md):
//   - a POST to IPVc answers the saved record, so the SerNr — the receipt id —
//     is readable from it;
//   - a confirmed receipt (OKFlag=1) can be neither edited nor deleted; it is
//     undone by booking its negative copy (ReverseReceipt);
//   - InvoiceNr and CustCode are ROW fields, which filter.<field> cannot reach,
//     so a customer's receipts are read by date and filtered here;
//   - updates_after / deletes_after exist only on a register with a
//     synchronisation index, which IPVc may not have on Excellent's hosting.
//     The sequence feed is therefore optional (ErrChangeFeedUnsupported), and
//     a deletes_after answer is believed only when it proves itself.

var (
	_ ReceiptCreator           = (*excellentProvider)(nil)
	_ PrepaymentReceiptApplier = (*excellentProvider)(nil)
	_ ReceiptGetter            = (*excellentProvider)(nil)
	_ InvoiceReceiptLister     = (*excellentProvider)(nil)
	_ PaymentChangeLister      = (*excellentProvider)(nil)
	_ ReceiptReverser          = (*excellentProvider)(nil)
)

var (
	// ErrPageShifted: a paged read found that the list moved between two pages
	// (the record that ended one page did not start the next), so the pages do
	// not join into one consistent list. Nothing from such a read is returned.
	ErrPageShifted = errors.New("provider list shifted between pages")
	// ErrReceiptNotReversible: ReverseReceipt refused before writing anything —
	// the receipt is a draft, invalidated, or has nothing that can be negated.
	ErrReceiptNotReversible = errors.New("receipt cannot be reversed")
)

const (
	// ebRowPrepayment is the row type a CUPNr (advance) row decodes to. The
	// vocabulary is SmartAccounts' (RowPrepayment), so callers classify both
	// providers' rows alike.
	ebRowPrepayment = "PREPAYMENT_PAYMENT"

	// ebReceiptPageSize is the page every paged IPVc read asks for.
	ebReceiptPageSize = 500
	// ebDefaultMaxPages caps ListInvoiceReceipts when the caller sets no cap.
	ebDefaultMaxPages = 5
	// ebChangeMaxPages caps each read of ListPaymentChanges (date window and
	// sequence feed alike): 40 pages of 500 receipts.
	ebChangeMaxPages = 40
)

// ebEarliest is the Since a ListInvoiceReceipts call without one reads from.
var ebEarliest = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

func (p *excellentProvider) receiptPageSize() int {
	if p.pageSize >= 2 {
		return p.pageSize
	}
	if p.pageSize > 0 {
		return 2 // a page must carry the overlap record and at least one new one
	}
	return ebReceiptPageSize
}

// --- Writes ---

// CreateReceipt books a receipt with CreatePayment's request plus, when set,
// the header Comment = input.Document (how the caller later recognises the
// receipt as its own) and the row Comment = input.Comment. It returns the
// SerNr Excellent Books answers; an answer without one is not a failure (the
// receipt exists, and its Document names it).
//
// ResolvedInvoice and ExpectInvoiceID are honoured as for SmartAccounts, with
// Excellent Books' invoice id being its SerNr: ResolvedInvoice.ID is the
// InvoiceNr booked against, and either one disagreeing with InvoiceNo refuses
// the receipt before anything is written (ErrProviderInvoiceMismatch).
// AccountKind is not read: the PayMode in BankID already names the bank or the
// cash desk.
func (p *excellentProvider) CreateReceipt(ctx context.Context, input CreatePaymentInput) (*CreatedReceipt, error) {
	const op = "CreateReceipt"
	if input.BankID == "" {
		return nil, p.wrapError(op, fmt.Errorf("%w: PayMode (BankID) is required for Excellent Books receipts", ErrInvalidInput))
	}
	invoiceNo, custCode := input.InvoiceNo, input.CustomerCode
	if ri := input.ResolvedInvoice; ri != nil {
		if ri.ID == "" {
			return nil, p.wrapError(op, fmt.Errorf("%w: ResolvedInvoice needs the invoice's SerNr", ErrInvalidInput))
		}
		if invoiceNo != "" && invoiceNo != ri.ID {
			return nil, p.wrapError(op, fmt.Errorf("%w: resolved invoice %s, invoice number %s", ErrProviderInvoiceMismatch, ri.ID, invoiceNo))
		}
		invoiceNo = ri.ID
		if custCode == "" {
			custCode = ri.CustomerID
		}
	}
	if invoiceNo == "" {
		return nil, p.wrapError(op, fmt.Errorf("%w: InvoiceNo is required", ErrInvalidInput))
	}
	if input.ExpectInvoiceID != "" && input.ExpectInvoiceID != invoiceNo {
		return nil, p.wrapError(op, fmt.Errorf("%w: invoice %s, expected %s", ErrProviderInvoiceMismatch, invoiceNo, input.ExpectInvoiceID))
	}

	in := input
	in.InvoiceNo, in.CustomerCode = invoiceNo, custCode
	fields := excellentPaymentFields(in)
	if input.Document != "" {
		fields["set_field.Comment"] = input.Document
	}
	if input.Comment != "" {
		fields["set_row_field.0.Comment"] = input.Comment
	}

	rec, err := p.client.PostReceipt(ctx, fields)
	if err != nil {
		return nil, p.wrapError(op, err)
	}
	out := &CreatedReceipt{
		Amount:     input.Amount,
		InvoiceID:  invoiceNo,
		CustomerID: fields["set_row_field.0.CustCode"],
	}
	if rec != nil {
		out.ID, out.Number = rec.SerNr, rec.SerNr
		if out.Amount.IsZero() {
			// RecVal was left to Excellent Books (CreatePayment omits a zero
			// amount); the saved record says what it booked.
			out.Amount = sumRecVal(rec.Rows)
		}
	}
	return out, nil
}

// ApplyPrepaymentReceipt is ApplyPrepayment that also writes the header
// Comment = input.Document (when set) and returns the application's SerNr.
// The amount it reports is the receipt's net, zero: the invoice row and the
// advance row offset each other. An answer without a SerNr is not a failure.
func (p *excellentProvider) ApplyPrepaymentReceipt(ctx context.Context, input ApplyPrepaymentInput) (*CreatedReceipt, error) {
	const op = "ApplyPrepaymentReceipt"
	if input.BankID == "" {
		return nil, p.wrapError(op, fmt.Errorf("%w: PayMode (BankID) is required for Excellent Books receipts", ErrInvalidInput))
	}
	if input.InvoiceNo == "" || input.PrepaymentNo == "" {
		return nil, p.wrapError(op, fmt.Errorf("%w: InvoiceNo and PrepaymentNo are required", ErrInvalidInput))
	}
	fields := excellentApplyFields(input)
	if input.Document != "" {
		fields["set_field.Comment"] = input.Document
	}
	rec, err := p.client.PostReceipt(ctx, fields)
	if err != nil {
		return nil, p.wrapError(op, err)
	}
	out := &CreatedReceipt{Amount: decimal.Zero, InvoiceID: input.InvoiceNo, CustomerID: input.CustomerCode}
	if rec != nil {
		out.ID, out.Number = rec.SerNr, rec.SerNr
	}
	return out, nil
}

// ReverseReceipt undoes a confirmed receipt the way Excellent prescribes: it
// reads the original and books its negative copy — confirmed (OKFlag=1), same
// PayMode, PayCurCode and header Objects, dated input.Date (or the original's
// date), header Comment = input.Document, and every row copied with its stp,
// InvoiceNr, CUPNr, CustCode and Objects, RecVal (and BankVal when the original
// has one) negated, and PayDate the reversal's date. A draft or invalidated
// receipt is refused before anything is written (ErrReceiptNotReversible): a
// draft is not in the books, and an invalidated one already counts as absent.
// The returned CreatedReceipt is the REVERSAL; its Amount is the negated total.
func (p *excellentProvider) ReverseReceipt(ctx context.Context, input ReverseReceiptInput) (*CreatedReceipt, error) {
	const op = "ReverseReceipt"
	if input.ID == "" {
		return nil, p.wrapError(op, fmt.Errorf("%w: ID is required", ErrInvalidInput))
	}
	orig, err := p.getReceiptRecord(ctx, op, input.ID)
	if err != nil {
		return nil, err
	}
	decoded := mapEBReceipt(*orig)
	refuse := func(why string, args ...any) (*CreatedReceipt, error) {
		return nil, p.wrapError(op, fmt.Errorf("%w: receipt %s %s", ErrReceiptNotReversible, input.ID, fmt.Sprintf(why, args...)))
	}
	switch {
	case decoded.Draft:
		return refuse("is not confirmed (OKFlag %q): a draft is not in the books", orig.OKFlag)
	case decoded.Void:
		return refuse("is invalidated (Invalid %q)", string(orig.Invalid))
	case len(orig.Rows) == 0:
		return refuse("has no rows")
	case orig.PayMode == "":
		return refuse("has no PayMode")
	}

	date := orig.TransDate
	if !input.Date.IsZero() {
		date = formatExcellentDate(input.Date)
	}
	if date == "" {
		return refuse("has no TransDate and no reversal date was given")
	}

	fields := map[string]string{
		"set_field.TransDate": date,
		"set_field.OKFlag":    "1",
		"set_field.PayMode":   orig.PayMode,
	}
	if orig.PayCurCode != "" {
		fields["set_field.PayCurCode"] = orig.PayCurCode
	}
	if orig.Objects != "" {
		fields["set_field.Objects"] = orig.Objects
	}
	if input.Document != "" {
		fields["set_field.Comment"] = input.Document
	}

	total := decimal.Zero
	invoices := map[string]bool{}
	for i, row := range orig.Rows {
		recVal, err := decimal.NewFromString(strings.TrimSpace(row.RecVal))
		if err != nil {
			return refuse("row %d has an unreadable RecVal %q", i, row.RecVal)
		}
		prefix := fmt.Sprintf("set_row_field.%d.", i)
		set := func(field, value string) {
			if value != "" {
				fields[prefix+field] = value
			}
		}
		set("stp", row.Stp)
		set("InvoiceNr", row.InvoiceNr)
		set("CUPNr", row.CUPNr)
		set("CustCode", row.CustCode)
		set("Objects", row.Objects)
		fields[prefix+"RecVal"] = recVal.Neg().String()
		if strings.TrimSpace(row.BankVal) != "" {
			bankVal, err := decimal.NewFromString(strings.TrimSpace(row.BankVal))
			if err != nil {
				return refuse("row %d has an unreadable BankVal %q", i, row.BankVal)
			}
			fields[prefix+"BankVal"] = bankVal.Neg().String()
		}
		fields[prefix+"PayDate"] = date
		total = total.Add(recVal.Neg())
		if row.InvoiceNr != "" {
			invoices[row.InvoiceNr] = true
		}
	}

	rec, err := p.client.PostReceipt(ctx, fields)
	if err != nil {
		return nil, p.wrapError(op, err)
	}
	out := &CreatedReceipt{Amount: total, CustomerID: decoded.CustomerID}
	if len(invoices) == 1 {
		for inv := range invoices {
			out.InvoiceID = inv
		}
	}
	if rec != nil {
		out.ID, out.Number = rec.SerNr, rec.SerNr
	}
	return out, nil
}

// --- Reads ---

// GetReceipt reads one receipt by SerNr. A 404, or a 2xx answer carrying no
// record at all, is an error for which IsNotFound is true. An answer that
// carries some other receipt is ErrIDFilterIgnored, never not-found: it says
// nothing about whether the asked-for receipt exists.
func (p *excellentProvider) GetReceipt(ctx context.Context, id string) (*Receipt, error) {
	const op = "GetReceipt"
	if id == "" {
		return nil, p.wrapError(op, fmt.Errorf("%w: id is required", ErrInvalidInput))
	}
	rec, err := p.getReceiptRecord(ctx, op, id)
	if err != nil {
		return nil, err
	}
	r := mapEBReceipt(*rec)
	return &r, nil
}

func (p *excellentProvider) getReceiptRecord(ctx context.Context, op, id string) (*excellentbooks.Receipt, error) {
	recs, err := p.client.GetReceiptRecords(ctx, id)
	if err != nil {
		return nil, p.wrapError(op, err)
	}
	if len(recs) == 0 {
		return nil, &ProviderError{Provider: "excellentbooks", Op: op,
			Err: fmt.Errorf("%w: receipt %s: Excellent Books answered with no record", ErrNotFound, id)}
	}
	for i := range recs {
		if recs[i].SerNr == id {
			return &recs[i], nil
		}
	}
	return nil, p.wrapError(op, fmt.Errorf("%w: asked for receipt %s, answered %q", ErrIDFilterIgnored, id, recs[0].SerNr))
}

// ListInvoiceReceipts reads the receipts dated Since..Until (Since zero ⇒
// 01.01.2000, Until zero ⇒ open-ended), page by page, and returns — whole —
// every receipt with a row for CustomerID (and, when set, on InvoiceID).
// Excellent Books cannot filter by a row field, so the date window is read in
// full and filtered here. A read unfinished at MaxPages (0 ⇒ 5) fails with
// ErrTooManyPages; one whose pages do not join fails with ErrPageShifted.
// Never a partial list.
func (p *excellentProvider) ListInvoiceReceipts(ctx context.Context, input ListInvoiceReceiptsInput) ([]Receipt, error) {
	const op = "ListInvoiceReceipts"
	if input.CustomerID == "" {
		return nil, p.wrapError(op, fmt.Errorf("%w: CustomerID is required", ErrInvalidInput))
	}
	maxPages := input.MaxPages
	if maxPages <= 0 {
		maxPages = ebDefaultMaxPages
	}
	since := input.Since
	if since.IsZero() {
		since = ebEarliest
	}
	recs, _, err := p.readReceiptWindow(ctx, since, input.Until, maxPages)
	if err != nil {
		return nil, p.wrapError(op, err)
	}
	out := make([]Receipt, 0)
	for _, r := range recs {
		if ebReceiptTouches(r, input.CustomerID, input.InvoiceID) {
			out = append(out, mapEBReceipt(r))
		}
	}
	return out, nil
}

// ebReceiptTouches reports whether any row of r is the customer's (and, when
// invoiceID is set, on that invoice).
func ebReceiptTouches(r excellentbooks.Receipt, customerID, invoiceID string) bool {
	for _, row := range r.Rows {
		if row.CustCode == customerID && (invoiceID == "" || row.InvoiceNr == invoiceID) {
			return true
		}
	}
	return false
}

// ListPaymentChanges reads receipts in one of two ways.
//
// Without SinceSequence it reads the date window Since..Until (Since required;
// Until zero ⇒ open-ended; Precise and ClientsOnly are not read — IPVc holds
// customer receipts only and its range is by date), page by page, capped at 40
// pages: unfinished ⇒ ErrTooManyPages, pages that do not join ⇒
// ErrPageShifted. Sequence is the FIRST page's data@sequence — the earliest
// moment of the read, so a change made while it ran is read again next time
// rather than skipped. Deleted is empty and DeletedComplete false: a window
// says nothing about deletions.
//
// With SinceSequence it reads updates_after=SinceSequence instead, paging by
// the last record's @sequence, whatever the receipts' dates. A refusal — an
// error payload, or a 4xx other than 401/403/408/429 — and an answer that
// shows the parameter was not applied (a record below the cursor, a full page
// that does not advance it) are ErrChangeFeedUnsupported; a transport error or
// a 5xx is an ordinary error. Sequence is the last page's data@sequence (else
// the last record's @sequence, else SinceSequence itself).
//
// WithDeletions (only with SinceSequence) also reads deletes_after, whose
// answer is undocumented. Its SerNrs reach Deleted only when the answer proves
// it is a deletion list: IPVc records each carrying a SerNr and a numeric
// @sequence not below the cursor, none of them among the live receipts the
// update read just returned. That last test is what tells an answer to
// deletes_after from a register that ignored the parameter and listed live
// receipts. DeletedComplete is true only for such an answer that also did not
// fill its page. A refusal of deletes_after leaves Deleted empty and
// DeletedComplete false, with no error.
//
// Receipts are decoded per mapEBReceipt; Payments is ListPayments' projection
// of the same receipts.
func (p *excellentProvider) ListPaymentChanges(ctx context.Context, input PaymentChangesInput) (*PaymentChanges, error) {
	const op = "ListPaymentChanges"
	if input.SinceSequence == "" {
		if input.Since.IsZero() {
			return nil, p.wrapError(op, fmt.Errorf("%w: Since or SinceSequence is required", ErrInvalidInput))
		}
		recs, seq, err := p.readReceiptWindow(ctx, input.Since, input.Until, ebChangeMaxPages)
		if err != nil {
			return nil, p.wrapError(op, err)
		}
		return ebPaymentChanges(recs, seq), nil
	}

	recs, seq, err := p.readReceiptFeed(ctx, op, input.SinceSequence)
	if err != nil {
		return nil, err
	}
	out := ebPaymentChanges(recs, seq)
	if input.WithDeletions {
		deleted, complete, err := p.readDeletedReceipts(ctx, op, input.SinceSequence, recs)
		if err != nil {
			return nil, err
		}
		out.Deleted, out.DeletedComplete = deleted, complete
	}
	return out, nil
}

func ebPaymentChanges(recs []excellentbooks.Receipt, seq string) *PaymentChanges {
	out := &PaymentChanges{
		Receipts: make([]Receipt, 0, len(recs)),
		Payments: make([]Payment, 0, len(recs)),
		Sequence: seq,
	}
	for _, r := range recs {
		out.Receipts = append(out.Receipts, mapEBReceipt(r))
		out.Payments = append(out.Payments, mapExcellentPayment(r))
	}
	return out
}

// readReceiptWindow reads every receipt dated since..until (until zero ⇒
// open-ended) in pages of receiptPageSize, sorted by TransDate. Each page after
// the first starts one record early, on the record that ended the previous
// page: when that record is not where it was, the list moved between the two
// requests (a receipt saved or removed earlier in the order) and a record may
// have been skipped, so the read starts over once and then fails with
// ErrPageShifted. It returns the receipts each once (the later copy of a
// SerNr wins) and the first page's data@sequence.
func (p *excellentProvider) readReceiptWindow(ctx context.Context, since, until time.Time, maxPages int) ([]excellentbooks.Receipt, string, error) {
	rng := formatExcellentDate(since) + ":"
	if !until.IsZero() {
		rng += formatExcellentDate(until)
	}
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		var recs []excellentbooks.Receipt
		var seq string
		recs, seq, err = p.readReceiptWindowOnce(ctx, rng, maxPages)
		if err == nil {
			return recs, seq, nil
		}
		if !errors.Is(err, ErrPageShifted) {
			return nil, "", err
		}
		slog.Warn("excellentbooks: receipt list shifted between pages; reading it again", "range", rng, "attempt", attempt+1, "error", err)
	}
	return nil, "", err
}

func (p *excellentProvider) readReceiptWindowOnce(ctx context.Context, rng string, maxPages int) ([]excellentbooks.Receipt, string, error) {
	size := p.receiptPageSize()
	var out []excellentbooks.Receipt
	index := map[string]int{}
	firstSeq := ""
	anchor := ""
	offset := 0
	for page := 0; ; page++ {
		if page == maxPages {
			return nil, "", fmt.Errorf("%w: IPVc range %s unfinished after %d pages of %d", ErrTooManyPages, rng, maxPages, size)
		}
		recs, seq, err := p.client.ListReceipts(ctx, excellentbooks.ListParams{Sort: "TransDate", Range: rng, Limit: size, Offset: offset})
		if err != nil {
			return nil, "", err
		}
		if page == 0 {
			firstSeq = seq
		}
		fresh := recs
		if page > 0 {
			if len(recs) == 0 || recs[0].SerNr != anchor {
				got := "nothing"
				if len(recs) > 0 {
					got = fmt.Sprintf("%q", recs[0].SerNr)
				}
				return nil, "", fmt.Errorf("%w: IPVc range %s page %d at offset %d starts with %s, expected receipt %q",
					ErrPageShifted, rng, page+1, offset, got, anchor)
			}
			fresh = recs[1:]
		}
		for _, r := range fresh {
			out = ebAddReceipt(out, index, r)
		}
		if len(recs) < size {
			return out, firstSeq, nil
		}
		anchor = recs[len(recs)-1].SerNr
		if anchor == "" {
			return nil, "", fmt.Errorf("%w: IPVc range %s page %d ends with a receipt without SerNr; pages cannot be joined", ErrPageShifted, rng, page+1)
		}
		offset += size - 1
	}
}

// readReceiptFeed reads updates_after=since to its end, paging by the last
// record's @sequence (the answer is ordered by sequence, and a receipt changed
// while the read runs moves to its end, so it is read rather than skipped).
func (p *excellentProvider) readReceiptFeed(ctx context.Context, op, since string) ([]excellentbooks.Receipt, string, error) {
	size := p.receiptPageSize()
	var out []excellentbooks.Receipt
	index := map[string]int{}
	cursor := since
	lastDataSeq, lastRecSeq := "", ""
	for page := 0; ; page++ {
		if page == ebChangeMaxPages {
			return nil, "", p.wrapError(op, fmt.Errorf("%w: IPVc updates_after=%s unfinished after %d pages of %d", ErrTooManyPages, since, ebChangeMaxPages, size))
		}
		recs, dataSeq, err := p.client.ListReceipts(ctx, excellentbooks.ListParams{UpdatesAfter: cursor, Limit: size})
		if err != nil {
			if ebFeedRefused(err) {
				return nil, "", &ProviderError{Provider: "excellentbooks", Op: op, Err: fmt.Errorf("%w: IPVc updates_after: %w", ErrChangeFeedUnsupported, err)}
			}
			return nil, "", p.wrapError(op, err)
		}
		for _, r := range recs {
			if seqBefore(r.Sequence, cursor) {
				return nil, "", p.feedNotApplied(op, "updates_after=%s answered receipt %s at sequence %s, below the cursor", cursor, r.SerNr, r.Sequence)
			}
			out = ebAddReceipt(out, index, r)
			if r.Sequence != "" {
				lastRecSeq = r.Sequence
			}
		}
		lastDataSeq = dataSeq
		if len(recs) < size {
			break
		}
		last := recs[len(recs)-1].Sequence
		if last == "" {
			return nil, "", p.feedNotApplied(op, "updates_after=%s answered a full page whose last receipt carries no @sequence", cursor)
		}
		if !seqAfter(last, cursor) {
			return nil, "", p.feedNotApplied(op, "updates_after=%s answered a full page ending at sequence %s, which does not advance it", cursor, last)
		}
		cursor = last
	}
	seq := lastDataSeq
	if seq == "" {
		seq = lastRecSeq
	}
	if seq == "" {
		seq = since
	}
	return out, seq, nil
}

func (p *excellentProvider) feedNotApplied(op, format string, args ...any) error {
	return &ProviderError{Provider: "excellentbooks", Op: op,
		Err: fmt.Errorf("%w: IPVc %s; the parameter was not applied", ErrChangeFeedUnsupported, fmt.Sprintf(format, args...))}
}

// readDeletedReceipts reads deletes_after=since and returns the deleted SerNrs
// and whether that list is complete — only for an answer that proves it is a
// deletion list (see ListPaymentChanges). Anything else answers no ids, not
// complete, and no error; only a transport error or a 5xx is an error.
func (p *excellentProvider) readDeletedReceipts(ctx context.Context, op, since string, live []excellentbooks.Receipt) ([]string, bool, error) {
	size := p.receiptPageSize()
	del, err := p.client.ListDeletedReceipts(ctx, since, size)
	if err != nil {
		if ebFeedRefused(err) {
			slog.Info("excellentbooks: IPVc refused deletes_after; deletions unknown", "since", since, "error", err)
			return nil, false, nil
		}
		return nil, false, p.wrapError(op, err)
	}
	unproven := func(why string, args ...any) ([]string, bool, error) {
		slog.Warn("excellentbooks: deletes_after answer not taken as a deletion list", "since", since, "reason", fmt.Sprintf(why, args...))
		return nil, false, nil
	}
	if !del.Recognised {
		return unproven("not a list of IPVc records with a SerNr each (%d bytes)", len(del.Raw))
	}
	sinceN, sinceOK := parseSeq(since)
	liveSerNrs := make(map[string]bool, len(live))
	for _, r := range live {
		liveSerNrs[r.SerNr] = true
	}
	ids := make([]string, 0, len(del.Records))
	for _, d := range del.Records {
		if liveSerNrs[d.SerNr] {
			return unproven("receipt %s is listed deleted and was just read as live", d.SerNr)
		}
		n, ok := parseSeq(d.Sequence)
		if !sinceOK || !ok {
			return unproven("receipt %s: sequence %q cannot be compared with the cursor %q", d.SerNr, d.Sequence, since)
		}
		if n < sinceN {
			return unproven("receipt %s at sequence %s is below the cursor %s", d.SerNr, d.Sequence, since)
		}
		ids = append(ids, d.SerNr)
	}
	return ids, del.Count < size, nil
}

// ebFeedRefused reports whether err is Excellent Books refusing a request (an
// error payload on a 2xx, or a 4xx that is not about credentials, timing or
// rate) rather than failing to answer it.
func ebFeedRefused(err error) bool {
	var apiErr *excellentbooks.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch code := apiErr.StatusCode; {
	case code >= 200 && code < 300:
		return true
	case code == 401, code == 403, code == 408, code == 429:
		return false
	case code >= 400 && code < 500:
		return true
	}
	return false
}

// ebAddReceipt appends r, or replaces an earlier copy of the same SerNr (a
// later read of a receipt is the newer one).
func ebAddReceipt(out []excellentbooks.Receipt, index map[string]int, r excellentbooks.Receipt) []excellentbooks.Receipt {
	if r.SerNr != "" {
		if i, ok := index[r.SerNr]; ok {
			out[i] = r
			return out
		}
		index[r.SerNr] = len(out)
	}
	return append(out, r)
}

func parseSeq(s string) (uint64, bool) {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	return n, err == nil
}

// seqBefore is true only when both are numeric and a < b.
func seqBefore(a, b string) bool {
	an, aok := parseSeq(a)
	bn, bok := parseSeq(b)
	return aok && bok && an < bn
}

// seqAfter is a > b when both are numeric, else a != b.
func seqAfter(a, b string) bool {
	an, aok := parseSeq(a)
	bn, bok := parseSeq(b)
	if aok && bok {
		return an > bn
	}
	return a != b
}

// --- Decoding ---

// mapEBReceipt decodes one IPVc record (contract: "Decoding an IPVc" in the
// backend's docs/EXCELLENTBOOKS_RECEIPT_OWNERSHIP.md):
//
//   - ID/Number = SerNr, Date = TransDate, AccountName = PayMode, Currency =
//     PayCurCode, Document = the header Comment verbatim, Sequence = @sequence;
//   - Draft when OKFlag is not "1"; Void when an Invalid field is present and
//     neither "" nor "0";
//   - a row whose stp is not "1" is not mapped and sets HasExtras;
//   - a row with an InvoiceNr is CLIENT_INVOICE (DocID InvoiceNr), one with a
//     CUPNr and no InvoiceNr is PREPAYMENT_PAYMENT (DocID CUPNr); Amount is the
//     signed RecVal, Description the row Comment. A row with both, neither, or
//     an unreadable RecVal is not mapped and sets DecodeError;
//   - Amount sums the mapped rows; AdvanceAmount sums the positive CUPNr rows;
//     AdvanceSourceIDs are the CUPNrs of the negative ones;
//   - CustomerID is the CustCode every row carries, "" when they differ.
func mapEBReceipt(r excellentbooks.Receipt) Receipt {
	out := Receipt{
		ID:          r.SerNr,
		Number:      r.SerNr,
		Document:    r.Comment,
		Date:        parseExcellentDate(r.TransDate),
		AccountName: r.PayMode,
		Currency:    r.PayCurCode,
		Draft:       r.OKFlag != "1",
		Sequence:    r.Sequence,
		Rows:        make([]ReceiptRow, 0, len(r.Rows)),
	}
	var problems []string
	switch inv := strings.TrimSpace(string(r.Invalid)); {
	case strings.HasPrefix(inv, "{") || strings.HasPrefix(inv, "["):
		problems = append(problems, fmt.Sprintf("Invalid has an unexpected shape %s", inv))
	case inv != "" && inv != "0":
		out.Void = true
	}

	custAgree := len(r.Rows) > 0
	for i, row := range r.Rows {
		if row.CustCode != r.Rows[0].CustCode {
			custAgree = false
		}
		if row.Stp != "1" {
			out.HasExtras = true
			continue
		}
		name := row.RowNumber
		if name == "" {
			name = strconv.Itoa(i)
		}
		amt, err := decimal.NewFromString(strings.TrimSpace(row.RecVal))
		if err != nil {
			problems = append(problems, fmt.Sprintf("row %s: unreadable RecVal %q", name, row.RecVal))
			continue
		}
		switch {
		case row.InvoiceNr != "" && row.CUPNr != "":
			problems = append(problems, fmt.Sprintf("row %s carries both InvoiceNr %s and CUPNr %s", name, row.InvoiceNr, row.CUPNr))
			continue
		case row.InvoiceNr != "":
			out.Rows = append(out.Rows, ReceiptRow{Type: saRowClientInvoice, DocID: row.InvoiceNr, Amount: amt, Description: row.Comment})
		case row.CUPNr != "":
			out.Rows = append(out.Rows, ReceiptRow{Type: ebRowPrepayment, DocID: row.CUPNr, Amount: amt, Description: row.Comment})
			if amt.IsPositive() {
				out.AdvanceAmount = out.AdvanceAmount.Add(amt)
			} else if amt.IsNegative() {
				out.AdvanceSourceIDs = append(out.AdvanceSourceIDs, row.CUPNr)
			}
		default:
			problems = append(problems, fmt.Sprintf("row %s carries neither InvoiceNr nor CUPNr", name))
			continue
		}
		out.Amount = out.Amount.Add(amt)
	}
	if custAgree {
		out.CustomerID = r.Rows[0].CustCode
	}
	out.DecodeError = strings.Join(problems, "; ")
	return out
}

func sumRecVal(rows []excellentbooks.ReceiptRow) decimal.Decimal {
	sum := decimal.Zero
	for _, row := range rows {
		v, err := decimal.NewFromString(strings.TrimSpace(row.RecVal))
		if err == nil {
			sum = sum.Add(v)
		}
	}
	return sum
}
