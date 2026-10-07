package smartaccounts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Receipt-level reads (v0.15.0).
//
// Everything in this file is new and is reached only by callers that opt into
// receipt-level behaviour. The v0.14.5 read path — ListPayments → getList →
// getPaginated → []PaymentItem — is deliberately not touched or reused: that
// path decodes every page in one json.Unmarshal, so a single odd row fails the
// whole list, and it discards `deleted`. Changing either would change what
// existing clubs' pulls see. These reads have their own paginator and their own
// lenient, per-row decode instead.

var (
	// ErrPartnerFilterIgnored means a per-customer payments read returned a
	// payment belonging to somebody else. SmartAccounts then did not apply the
	// partnerId filter, and the result is neither "this customer's receipts" nor
	// complete; nothing may be concluded from it — least of all that a receipt
	// is missing.
	ErrPartnerFilterIgnored = errors.New("smartaccounts: partner filter ignored — the read returned another customer's payments")
	// ErrTooManyPages means a capped read still had more pages at its cap. The
	// items read so far are not the whole answer.
	ErrTooManyPages = errors.New("smartaccounts: read exceeded its page cap")
	// ErrIDFilterIgnored means a by-id payments read answered something other
	// than that one payment: another payment, or more than one page. The id
	// filter was then not applied, and the answer says nothing about whether the
	// payment exists — least of all that it was deleted.
	ErrIDFilterIgnored = errors.New("smartaccounts: id filter ignored — the by-id read answered other payments")
	// ErrUnreadablePayment means a per-customer read met a payment whose core
	// fields could not be decoded. Whose payment it is cannot be told, so the
	// read is neither provably this customer's nor complete. Kept apart from
	// ErrPartnerFilterIgnored so a decode problem is not reported as SmartAccounts
	// ignoring partnerId.
	ErrUnreadablePayment = errors.New("smartaccounts: payment could not be decoded")
)

// preciseDateFormat is SmartAccounts' second-precision date filter format.
const preciseDateFormat = "02.01.2006_15:04:05"

// tallinnTime converts t to Estonian local time, with the same tzdata-less
// fallback estonianNow uses for the request signature.
func tallinnTime(t time.Time) time.Time {
	if tallinnTZ != nil {
		return t.In(tallinnTZ)
	}
	offset := 2 * 60 * 60
	if isEUSummerTime(t.UTC()) {
		offset = 3 * 60 * 60
	}
	return t.In(time.FixedZone("EE", offset))
}

// TallinnDate formats t as the Tallinn calendar date ("dd.MM.yyyy"), the date
// SmartAccounts files a document under; "" for a zero t.
func TallinnDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return tallinnTime(t).Format("02.01.2006")
}

// PaymentChangeItem is one payment from a receipt-level read. It embeds the
// v0.14.5 PaymentItem (whose fields and tags are unchanged) and adds the fields
// only the new reads look at.
type PaymentChangeItem struct {
	PaymentItem
	// OrderNumber is the cash-desk order number; SmartAccounts sends it as a
	// string or a number (SANDBOX checklist item 3), so it is kept raw here and
	// read with OrderNumberString.
	OrderNumber json.RawMessage `json:"orderNumber"`
	// Comment is present only when the read asked for fetchComments.
	Comment string `json:"comment"`
	// PrepaymentRaw is the top-level `prepayment` object (an advance parked by
	// this payment), kept raw so a malformed one cannot fail the row's decode.
	PrepaymentRaw json.RawMessage `json:"prepayment"`

	// ExtrasRaw is the `extras` array — extra GL rows (price × quantity on an
	// account) an accountant can add to a payment. Kept raw: only its presence
	// matters here, because deleting a payment deletes those postings with it.
	ExtrasRaw json.RawMessage `json:"extras"`

	// Prepayment is PrepaymentRaw parsed into the existing request-side shape;
	// nil when the payment parked no advance or the object could not be read.
	Prepayment *PaymentPrepayment `json:"-"`
	// DecodeError is non-empty when some part of this payment could not be
	// read. The row is still returned — losing it would hide money — but a
	// caller must never treat what it does say as an allocation of money.
	DecodeError string `json:"-"`
	// Unreadable is true when even the payment's core fields could not be
	// decoded: only ID (when readable) and DecodeError are set.
	Unreadable bool `json:"-"`
}

// HasExtras reports whether the payment carries extra rows. Anything but an
// absent, null or empty `extras` counts — an unreadable one included, since
// "cannot tell" must never read as "none".
func (p PaymentChangeItem) HasExtras() bool {
	t := bytes.TrimSpace(p.ExtrasRaw)
	if len(t) == 0 || string(t) == "null" {
		return false
	}
	var elems []json.RawMessage
	if err := json.Unmarshal(t, &elems); err != nil {
		return true
	}
	return len(elems) > 0
}

// OrderNumberString returns the cash-desk order number as text, "" when absent.
func (p PaymentChangeItem) OrderNumberString() string {
	s, _ := rawScalar(p.OrderNumber)
	return s
}

// CashAccountItem is a cash desk (settings/cashaccounts). Keyed by name, like a
// bank account; a payment books into it with accountType=CASH.
type CashAccountItem struct {
	Name     string `json:"name"`
	Account  string `json:"account"` // GL code
	Currency string `json:"currency"`
}

// changePage is the envelope of one receipt-level page. `deleted` is kept raw
// and decoded by decodeDeleted, which tolerates string and number ids.
type changePage struct {
	HasMoreEntries bool            `json:"hasMoreEntries"`
	Deleted        json.RawMessage `json:"deleted"`
}

// ListPaymentChanges reads the payments feed described by params across all
// pages and returns its payments, the ids SmartAccounts reports deleted in the
// window, and whether that deleted list can be trusted to be complete.
//
// complete is true when page 1 came back as a JSON object, with or without a
// `deleted` key. Verified against a live company (07.10.2026): SmartAccounts
// OMITS the key when the window holds no deletion and sends it, on page 1,
// as soon as one exists — so a missing key on a readable page means "none",
// not "unknown". Reading it as unknown made every ordinary pull skip its
// receipt step. Only an empty or non-object body leaves complete false. A
// caller may act on an id that IS in deleted; on an id's ABSENCE only when
// complete is true.
//
// The feed is de-duplicated by payment id (the last copy read wins): offset
// pagination over a modifydate window repeats a payment edited mid-read.
func (c *Client) ListPaymentChanges(ctx context.Context, params ListPaymentsParams) (items []PaymentChangeItem, deleted []string, complete bool, err error) {
	items, deleted, complete, err = c.readPaymentPages(ctx, params.values(), maxFeedPages)
	if err != nil {
		return nil, nil, false, err
	}
	return dedupePaymentChanges(items), deleted, complete, nil
}

// GetPayment reads one payment by id, rows and comment included. An empty
// answer is *APIError{404}. An answer holding any other payment, or running past
// one page, means SA ignored the id filter: ErrIDFilterIgnored, never "not
// found" — absence is evidence of deletion only when the question was really
// asked.
//
// dateFrom=01.01.2000 goes with the id, as in FindInvoiceByNumber: SA refuses
// an unbounded clientinvoices:get unless id is given, and nobody has checked
// whether payments:get quietly applies a default date window when dateFrom is
// missing. Under such a window an old receipt that still exists would come back
// empty and read as deleted. The bound costs nothing if SA honours id.
//
// SANDBOX (checklist item 7): what SA answers for a deleted id (empty, 404, or
// an error with a message), and the positive control — an existing receipt over
// a year old, with and without dateFrom, must come back. Until both are
// recorded a caller must not act on not-found alone (see ReceiptGetter).
func (c *Client) GetPayment(ctx context.Context, id string) (*PaymentChangeItem, error) {
	if id == "" {
		return nil, fmt.Errorf("smartaccounts: GetPayment: empty id")
	}
	params := ListPaymentsParams{DateFrom: "01.01.2000", ID: id, FetchRows: true, FetchComments: true}
	items, _, _, err := c.readPaymentPages(ctx, params.values(), 1)
	if err != nil {
		if errors.Is(err, ErrTooManyPages) {
			return nil, fmt.Errorf("%w: GetPayment %s answered more than one page: %w", ErrIDFilterIgnored, id, err)
		}
		return nil, err
	}
	var hit *PaymentChangeItem
	for i := range items {
		if items[i].ID != id {
			return nil, fmt.Errorf("%w: GetPayment %s answered payment %s", ErrIDFilterIgnored, id, items[i].ID)
		}
		hit = &items[i]
	}
	if hit == nil {
		return nil, &APIError{StatusCode: 404, Message: "payment not found: " + id}
	}
	return hit, nil
}

// ListCustomerPayments reads one client's payments (partnerType=CLIENT,
// partnerId) dated from..to, rows and comments included, reading at most
// maxPages pages (0 ⇒ 5). from/to are sent as Tallinn calendar dates, the
// dates SA keeps; a zero from is 01.01.2000 and a zero to sends no dateTo.
//
// It refuses rather than guesses: a row belonging to any other partner aborts
// with ErrPartnerFilterIgnored (SANDBOX checklist item 4 — whether SA honours
// partnerId at all), a row whose owner cannot be read aborts with
// ErrUnreadablePayment, and a read still unfinished at maxPages aborts with
// ErrTooManyPages. The cap keeps a single admin click from spending a shared
// company's daily budget.
func (c *Client) ListCustomerPayments(ctx context.Context, partnerID string, from, to time.Time, maxPages int) ([]PaymentChangeItem, error) {
	if partnerID == "" {
		return nil, fmt.Errorf("smartaccounts: ListCustomerPayments: empty partner id")
	}
	if maxPages <= 0 {
		maxPages = 5
	}
	dateFrom := "01.01.2000"
	if !from.IsZero() {
		dateFrom = TallinnDate(from)
	}
	params := ListPaymentsParams{
		DateFrom:      dateFrom,
		PartnerType:   PartnerClient,
		PartnerID:     partnerID,
		FetchRows:     true,
		FetchComments: true,
	}
	if !to.IsZero() {
		params.DateTo = TallinnDate(to)
	}
	items, _, _, err := c.readPaymentPages(ctx, params.values(), maxPages)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if it.Unreadable {
			return nil, fmt.Errorf("%w (payment %q in %s's receipts: %s)", ErrUnreadablePayment, it.ID, partnerID, it.DecodeError)
		}
		if !belongsToClient(it, partnerID) {
			return nil, fmt.Errorf("%w (asked for %s, got payment %s)", ErrPartnerFilterIgnored, partnerID, it.ID)
		}
	}
	return dedupePaymentChanges(items), nil
}

// belongsToClient reports whether a payment is provably the given client's:
// a CLIENT payment whose every stated client id is that client, with at least
// one stated.
func belongsToClient(it PaymentChangeItem, clientID string) bool {
	if it.PartnerType != "" && it.PartnerType != PartnerClient {
		return false
	}
	stated := false
	if it.ClientID != "" {
		if it.ClientID != clientID {
			return false
		}
		stated = true
	}
	if it.Client != nil && it.Client.ID != "" {
		if it.Client.ID != clientID {
			return false
		}
		stated = true
	}
	return stated
}

// ListCashAccounts returns the company's cash desks (settings/cashaccounts).
// A 401/403 comes back as *APIError like every other call, so a caller can tell
// "the API user lacks the right" apart from "there are none".
func (c *Client) ListCashAccounts(ctx context.Context) ([]CashAccountItem, error) {
	var items []CashAccountItem
	if err := c.getAll(ctx, "settings/cashaccounts:get", nil, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// nettingBankItem is the decode target of ListNettingBankNames only. The
// existing BankAccountItem (ListBankAccounts, ListBanks) is not touched: forNetting
// is read here, raw and leniently, so an unexpected shape of that one field can
// never fail the bank list every push and setup screen already depends on.
type nettingBankItem struct {
	Name       string          `json:"name"`
	ForNetting json.RawMessage `json:"forNetting"`
}

// ListNettingBankNames returns the names of the bank accounts the company marks
// forNetting — SmartAccounts' own default for netting credit invoices against
// invoices. A separate read of settings/bankaccounts (one request) so the old
// bank list keeps its v0.14.5 decode; a forNetting value it cannot read counts
// as false.
func (c *Client) ListNettingBankNames(ctx context.Context) ([]string, error) {
	var items []nettingBankItem
	if err := c.getAll(ctx, "settings/bankaccounts:get", nil, &items); err != nil {
		return nil, err
	}
	var names []string
	for _, it := range items {
		if it.Name != "" && rawTrue(it.ForNetting) {
			names = append(names, it.Name)
		}
	}
	return names, nil
}

// rawTrue reads a JSON flag that may arrive as true, "true", 1 or "1".
func rawTrue(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	if string(t) == "true" {
		return true
	}
	s, ok := rawScalar(t)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1":
		return true
	}
	return false
}

// BudgetUsed returns how many requests this process has sent for this company
// in the trailing 24 hours. A floor, never the whole: other replicas and other
// tools on the same key are invisible to it. A hint for logs and screens only.
func (c *Client) BudgetUsed() int {
	if c.budget == nil {
		return 0
	}
	return c.budget.used(time.Now())
}

// maxFeedPages bounds an unbounded feed read the way getPaginated bounds the
// old one.
const maxFeedPages = 1000

// readPaymentPages drives payments:get across pages up to maxPages. Each page is
// decoded item by item, so one malformed payment becomes one row carrying a
// DecodeError instead of failing the read. deleted/complete describe page 1.
func (c *Client) readPaymentPages(ctx context.Context, params url.Values, maxPages int) (items []PaymentChangeItem, deleted []string, complete bool, err error) {
	if params == nil {
		params = url.Values{}
	}
	for pageNum := 1; pageNum <= maxPages; pageNum++ {
		params.Set("pageNumber", strconv.Itoa(pageNum))
		var raw json.RawMessage
		if err := c.get(ctx, "purchasesales/payments:get", params, &raw); err != nil {
			return nil, nil, false, err
		}
		if len(bytes.TrimSpace(raw)) == 0 {
			// No body: nothing more to read, and on page 1 nothing proven about
			// deletions.
			return items, deleted, complete, nil
		}
		var meta changePage
		if bytes.TrimSpace(raw)[0] == '{' {
			if err := json.Unmarshal(raw, &meta); err != nil {
				return nil, nil, false, fmt.Errorf("smartaccounts: unmarshal page meta: %w", err)
			}
		}
		if pageNum == 1 {
			deleted, complete, err = decodeDeleted(meta.Deleted)
			if err != nil {
				return nil, nil, false, err
			}
			// A readable object page without the key: no deletions (see
			// ListPaymentChanges). A non-object body proves nothing.
			if !complete && bytes.TrimSpace(raw)[0] == '{' && len(bytes.TrimSpace(meta.Deleted)) == 0 {
				complete = true
			}
		}
		arr, err := extractList(raw)
		if err != nil {
			return nil, nil, false, err
		}
		var rows []json.RawMessage
		if err := json.Unmarshal(arr, &rows); err != nil {
			return nil, nil, false, fmt.Errorf("smartaccounts: unmarshal list items: %w", err)
		}
		for _, r := range rows {
			items = append(items, decodePaymentChange(r))
		}
		if !meta.HasMoreEntries {
			return items, deleted, complete, nil
		}
	}
	return nil, nil, false, fmt.Errorf("%w (%d pages of payments:get)", ErrTooManyPages, maxPages)
}

// decodeDeleted reads page 1's `deleted`. SmartAccounts sends each element as
// an object — {"accountType":"BANK","date":"07.10.2026","id":"<guid>","number":2}
// (live, 07.10.2026) — whose id is taken; string and number elements are read
// as ids too. Any other shape fails this read only. Absent or null ⇒ (nil, false).
func decodeDeleted(raw json.RawMessage) ([]string, bool, error) {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || string(t) == "null" {
		return nil, false, nil
	}
	var elems []json.RawMessage
	if err := json.Unmarshal(t, &elems); err != nil {
		return nil, false, fmt.Errorf("smartaccounts: unreadable deleted list: %w", err)
	}
	out := make([]string, 0, len(elems))
	for _, e := range elems {
		if t := bytes.TrimSpace(e); len(t) > 0 && t[0] == '{' {
			var obj struct {
				ID json.RawMessage `json:"id"`
			}
			if err := json.Unmarshal(t, &obj); err == nil {
				e = obj.ID
			}
		}
		s, ok := rawScalar(e)
		if !ok || s == "" {
			return nil, false, fmt.Errorf("smartaccounts: unreadable deleted id %s", string(e))
		}
		out = append(out, s)
	}
	return out, true, nil
}

// rawScalar reads a JSON string or number as text. ok is false for any other
// shape; absent/null is ("", true).
func rawScalar(raw json.RawMessage) (string, bool) {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || string(t) == "null" {
		return "", true
	}
	if t[0] == '"' {
		var s string
		if err := json.Unmarshal(t, &s); err != nil {
			return "", false
		}
		return s, true
	}
	var n json.Number
	if err := json.Unmarshal(t, &n); err != nil {
		return "", false
	}
	return n.String(), true
}

// decodePaymentChange decodes one payment leniently. A payment whose core
// fields do not decode is kept with only its id (when even that can be read)
// and a DecodeError, so a caller sees that something is there.
func decodePaymentChange(raw json.RawMessage) PaymentChangeItem {
	var it PaymentChangeItem
	if err := json.Unmarshal(raw, &it); err != nil {
		var idOnly struct {
			ID json.RawMessage `json:"id"`
		}
		_ = json.Unmarshal(raw, &idOnly)
		id, _ := rawScalar(idOnly.ID)
		slog.Warn("smartaccounts: payment could not be decoded; kept as unreadable", "payment_id", id, "error", err)
		return PaymentChangeItem{PaymentItem: PaymentItem{ID: id}, DecodeError: err.Error(), Unreadable: true}
	}
	if t := bytes.TrimSpace(it.PrepaymentRaw); len(t) > 0 && string(t) != "null" {
		var pp PaymentPrepayment
		if err := json.Unmarshal(t, &pp); err != nil {
			it.DecodeError = "prepayment: " + err.Error()
		} else {
			it.Prepayment = &pp
		}
	}
	if t := bytes.TrimSpace(it.ExtrasRaw); len(t) > 0 && string(t) != "null" && t[0] != '[' {
		it.DecodeError = joinDecodeErr(it.DecodeError, "extras: not an array")
	}
	if _, ok := rawScalar(it.OrderNumber); !ok {
		it.DecodeError = joinDecodeErr(it.DecodeError, "orderNumber: not a string or number")
	}
	if it.DecodeError != "" {
		slog.Warn("smartaccounts: payment partly unreadable", "payment_id", it.ID, "error", it.DecodeError)
	}
	return it
}

func joinDecodeErr(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

// dedupePaymentChanges keeps one copy per payment id — the last one read — in
// the position its id was first seen. Rows with no id are all kept.
func dedupePaymentChanges(items []PaymentChangeItem) []PaymentChangeItem {
	pos := make(map[string]int, len(items))
	out := make([]PaymentChangeItem, 0, len(items))
	for _, it := range items {
		if it.ID == "" {
			out = append(out, it)
			continue
		}
		if i, seen := pos[it.ID]; seen {
			out[i] = it
			continue
		}
		pos[it.ID] = len(out)
		out = append(out, it)
	}
	return out
}
