package smartaccounts

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// pageServer answers payments:get page N (1-based) from pages, and anything
// else with "{}". It counts requests.
func pageServer(t *testing.T, pages []string, n *int32, queries *[]string) *Client {
	t.Helper()
	return newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(n, 1)
		if queries != nil {
			*queries = append(*queries, r.URL.RawQuery)
		}
		page := 1
		if p := r.URL.Query().Get("pageNumber"); p != "" {
			page = int(p[0] - '0')
		}
		w.Header().Set("Content-Type", "application/json")
		if page-1 < len(pages) {
			_, _ = w.Write([]byte(pages[page-1]))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
}

func TestListPaymentsParamsZeroNewFieldsUnchanged(t *testing.T) {
	// Every v0.14.5 field set, every v0.15.0 field zero: the encoded query must
	// be exactly what v0.14.5 encoded.
	p := ListPaymentsParams{
		DateFrom: "01.07.2026", DateTo: "31.07.2026", DateType: "modifydate",
		PartnerType: "CLIENT", AccountType: "BANK", PartnerID: "cli-9", FetchRows: true,
	}
	const want = "accountType=BANK&dateFrom=01.07.2026&dateTo=31.07.2026&dateType=modifydate&fetchRows=true&partnerId=cli-9&partnerType=CLIENT"
	if got := p.values().Encode(); got != want {
		t.Fatalf("values() = %s\nwant      %s", got, want)
	}
	if got := (ListPaymentsParams{}).values().Encode(); got != "" {
		t.Fatalf("zero params must encode to nothing, got %q", got)
	}
}

func TestListPaymentsParamsPreciseTallinn(t *testing.T) {
	cases := []struct {
		in   time.Time
		want string
	}{
		{time.Date(2026, 10, 7, 9, 30, 15, 0, time.UTC), "07.10.2026_12:30:15"}, // EEST, UTC+3
		{time.Date(2026, 1, 15, 22, 0, 1, 0, time.UTC), "16.01.2026_00:00:01"},  // EET, UTC+2, crosses midnight
	}
	for _, c := range cases {
		p := ListPaymentsParams{DateFrom: "01.01.2000", DateFromPrecise: c.in}
		if got := p.values().Get("dateFrom"); got != c.want {
			t.Errorf("DateFromPrecise %s → %q, want %q", c.in, got, c.want)
		}
	}
	// The other new fields reach the wire under their own keys.
	v := ListPaymentsParams{ID: "p1", AccountName: "Kassa", PartnerName: "Mari", FetchComments: true}.values()
	if v.Encode() != "accountName=Kassa&fetchComments=true&id=p1&partnerName=Mari" {
		t.Errorf("new fields encode as %s", v.Encode())
	}
}

func TestListPaymentChangesLenientDeleted(t *testing.T) {
	cases := []struct {
		name         string
		page         string
		wantDeleted  []string
		wantComplete bool
		wantErr      bool
	}{
		{"strings", `{"payments":[],"deleted":["a","b"],"hasMoreEntries":false}`, []string{"a", "b"}, true, false},
		{"numbers", `{"payments":[],"deleted":[12,345],"hasMoreEntries":false}`, []string{"12", "345"}, true, false},
		{"mixed", `{"payments":[],"deleted":["a",7],"hasMoreEntries":false}`, []string{"a", "7"}, true, false},
		{"empty list", `{"payments":[],"deleted":[],"hasMoreEntries":false}`, []string{}, true, false},
		{"key missing", `{"payments":[],"hasMoreEntries":false}`, nil, false, false},
		{"null", `{"payments":[],"deleted":null,"hasMoreEntries":false}`, nil, false, false},
		{"missing body", ``, nil, false, false},
		{"object element", `{"payments":[],"deleted":[{"id":"a"}],"hasMoreEntries":false}`, nil, false, true},
		{"not a list", `{"payments":[],"deleted":"a","hasMoreEntries":false}`, nil, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c2 := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(c.page))
			})
			_, deleted, complete, err := c2.ListPaymentChanges(context.Background(), ListPaymentsParams{DateFrom: "01.01.2026", DateType: "modifydate"})
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if c.wantErr {
				return
			}
			if complete != c.wantComplete {
				t.Errorf("complete = %v, want %v", complete, c.wantComplete)
			}
			if strings.Join(deleted, ",") != strings.Join(c.wantDeleted, ",") || (deleted == nil) != (c.wantDeleted == nil) {
				t.Errorf("deleted = %#v, want %#v", deleted, c.wantDeleted)
			}
		})
	}
}

func TestListPaymentChangesOnlyPageOneDeletedCounts(t *testing.T) {
	var n int32
	c := pageServer(t, []string{
		`{"payments":[{"id":"a","amount":1}],"deleted":["x"],"hasMoreEntries":true}`,
		`{"payments":[{"id":"b","amount":2}],"deleted":["y"],"hasMoreEntries":false}`,
	}, &n, nil)
	items, deleted, complete, err := c.ListPaymentChanges(context.Background(), ListPaymentsParams{DateFrom: "01.01.2026"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || strings.Join(deleted, ",") != "x" || !complete || n != 2 {
		t.Fatalf("items=%d deleted=%v complete=%v requests=%d", len(items), deleted, complete, n)
	}
}

func TestListPaymentChangesDedupesByID(t *testing.T) {
	var n int32
	c := pageServer(t, []string{
		`{"payments":[{"id":"A","amount":10},{"id":"B","amount":20}],"deleted":[],"hasMoreEntries":true}`,
		`{"payments":[{"id":"A","amount":11},{"id":"C","amount":30}],"hasMoreEntries":false}`,
	}, &n, nil)
	items, _, _, err := c.ListPaymentChanges(context.Background(), ListPaymentsParams{DateFrom: "01.01.2026"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range items {
		got = append(got, it.ID+"="+it.Amount.String())
	}
	if strings.Join(got, " ") != "A=11 B=20 C=30" {
		t.Fatalf("dedupe = %v, want A=11 (last copy, first position) B=20 C=30", got)
	}
}

func TestListPaymentChangesMalformedRowIsUnknownNotError(t *testing.T) {
	var n int32
	c := pageServer(t, []string{`{"payments":[` +
		`{"id":"good","amount":5,"rows":[{"amount":5,"type":"CLIENT_INVOICE","id":"inv-1"}]},` +
		`{"id":"bad-core","amount":{"x":1}},` +
		`{"id":"bad-prepay","amount":5,"prepayment":"abc"},` +
		`{"id":"bad-order","amount":5,"orderNumber":{"n":1}},` +
		`{"id":"cash","amount":5,"orderNumber":17,"prepayment":{"amount":2,"account":"21211"},"comment":"hi"}` +
		`],"deleted":[],"hasMoreEntries":false}`}, &n, nil)
	items, _, complete, err := c.ListPaymentChanges(context.Background(), ListPaymentsParams{DateFrom: "01.01.2026"})
	if err != nil {
		t.Fatalf("one odd row must not fail the feed: %v", err)
	}
	if !complete || len(items) != 5 {
		t.Fatalf("complete=%v items=%d", complete, len(items))
	}
	by := map[string]PaymentChangeItem{}
	for _, it := range items {
		by[it.ID] = it
	}
	if g := by["good"]; g.DecodeError != "" || g.Unreadable || len(g.Rows) != 1 {
		t.Errorf("good row: %+v", g)
	}
	if b := by["bad-core"]; !b.Unreadable || b.DecodeError == "" {
		t.Errorf("bad core must be kept as unreadable: %+v", b)
	}
	if b := by["bad-prepay"]; b.Unreadable || b.DecodeError == "" || b.Prepayment != nil || !b.Amount.Equal(dec("5")) {
		t.Errorf("bad prepayment: %+v", b)
	}
	if b := by["bad-order"]; b.Unreadable || b.DecodeError == "" {
		t.Errorf("bad orderNumber: %+v", b)
	}
	cash := by["cash"]
	if cash.DecodeError != "" || cash.OrderNumberString() != "17" || cash.Prepayment == nil ||
		!cash.Prepayment.Amount.Equal(dec("2")) || cash.Prepayment.Account != "21211" || cash.Comment != "hi" {
		t.Errorf("cash row: %+v", cash)
	}
}

func TestGetPayment(t *testing.T) {
	var queries []string
	var n int32
	c := pageServer(t, []string{`{"payments":[{"id":"p1","amount":5,"document":"KLUBIO-PMT-9"}],"hasMoreEntries":false}`}, &n, &queries)
	got, err := c.GetPayment(context.Background(), "p1")
	if err != nil || got.ID != "p1" || got.Document != "KLUBIO-PMT-9" {
		t.Fatalf("GetPayment = %+v, %v", got, err)
	}
	if q := stripSigned(queries[0]); q != "apikey=pub-key&fetchComments=true&fetchRows=true&id=p1&pageNumber=1" {
		t.Errorf("query = %s", q)
	}

	empty := pageServer(t, []string{`{"payments":[],"hasMoreEntries":false}`}, new(int32), nil)
	_, err = empty.GetPayment(context.Background(), "p1")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Fatalf("empty answer must be 404, got %v", err)
	}

	other := pageServer(t, []string{`{"payments":[{"id":"p2","amount":5}],"hasMoreEntries":false}`}, new(int32), nil)
	_, err = other.GetPayment(context.Background(), "p1")
	if err == nil || (errors.As(err, &apiErr) && apiErr.StatusCode == 404) {
		t.Fatalf("an answer holding another payment must be an error, never not-found: %v", err)
	}

	paged := pageServer(t, []string{`{"payments":[{"id":"p1","amount":5}],"hasMoreEntries":true}`}, new(int32), nil)
	if _, err = paged.GetPayment(context.Background(), "p1"); err == nil || (errors.As(err, &apiErr) && apiErr.StatusCode == 404) {
		t.Fatalf("a paged by-id answer means the filter was ignored: %v", err)
	}
}

func stripSigned(raw string) string {
	var kept []string
	for _, p := range strings.Split(raw, "&") {
		if strings.HasPrefix(p, "timestamp=") || strings.HasPrefix(p, "signature=") {
			continue
		}
		kept = append(kept, p)
	}
	return strings.Join(kept, "&")
}

func TestListCustomerPaymentsQueryAndFilter(t *testing.T) {
	var queries []string
	var n int32
	c := pageServer(t, []string{`{"payments":[` +
		`{"id":"p1","partnerType":"CLIENT","clientId":"cli-9","client":{"id":"cli-9"},"amount":5},` +
		`{"id":"p2","partnerType":"CLIENT","client":{"id":"cli-9"},"amount":6}` +
		`],"hasMoreEntries":false}`}, &n, &queries)
	items, err := c.ListCustomerPayments(context.Background(), "cli-9", time.Time{}, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), 0)
	if err != nil || len(items) != 2 {
		t.Fatalf("items=%d err=%v", len(items), err)
	}
	if q := stripSigned(queries[0]); q != "apikey=pub-key&dateFrom=01.01.2000&dateTo=01.10.2026&fetchComments=true&fetchRows=true&pageNumber=1&partnerId=cli-9&partnerType=CLIENT" {
		t.Errorf("query = %s", q)
	}
}

func TestListCustomerPaymentsAbortsOnForeignClient(t *testing.T) {
	for name, row := range map[string]string{
		"other client":      `{"id":"p2","partnerType":"CLIENT","client":{"id":"cli-1"},"amount":6}`,
		"clientId disagree": `{"id":"p2","partnerType":"CLIENT","clientId":"cli-1","client":{"id":"cli-9"},"amount":6}`,
		"no client stated":  `{"id":"p2","partnerType":"CLIENT","amount":6}`,
		"vendor":            `{"id":"p2","partnerType":"VENDOR","vendor":{"id":"cli-9"},"amount":6}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := pageServer(t, []string{`{"payments":[{"id":"p1","partnerType":"CLIENT","client":{"id":"cli-9"},"amount":5},` + row + `],"hasMoreEntries":false}`}, new(int32), nil)
			_, err := c.ListCustomerPayments(context.Background(), "cli-9", time.Time{}, time.Time{}, 5)
			if !errors.Is(err, ErrPartnerFilterIgnored) {
				t.Fatalf("want ErrPartnerFilterIgnored, got %v", err)
			}
		})
	}
}

func TestListCustomerPaymentsPageCap(t *testing.T) {
	page := `{"payments":[{"id":"p","partnerType":"CLIENT","client":{"id":"cli-9"},"amount":5}],"hasMoreEntries":true}`
	var n int32
	c := pageServer(t, []string{page, page, page}, &n, nil)
	_, err := c.ListCustomerPayments(context.Background(), "cli-9", time.Time{}, time.Time{}, 2)
	if !errors.Is(err, ErrTooManyPages) {
		t.Fatalf("want ErrTooManyPages, got %v", err)
	}
	if n != 2 {
		t.Fatalf("cap 2 must stop at 2 requests, sent %d", n)
	}
}

func TestListCashAccounts(t *testing.T) {
	var path string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"cashAccounts":[{"name":"Kassa","account":"1000","currency":"EUR"}]}`))
	})
	got, err := c.ListCashAccounts(context.Background())
	if err != nil || len(got) != 1 || got[0].Name != "Kassa" || got[0].Account != "1000" || got[0].Currency != "EUR" {
		t.Fatalf("ListCashAccounts = %+v, %v", got, err)
	}
	if path != "/en/api/settings/cashaccounts:get" {
		t.Errorf("path = %s", path)
	}

	denied := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"no rights"}`, http.StatusForbidden)
	})
	_, err = denied.ListCashAccounts(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("a missing privilege must come back as *APIError 403, got %v", err)
	}
}

func TestBudgetUsed(t *testing.T) {
	resetSharedBudgets()
	t.Cleanup(resetSharedBudgets)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"vatPcs":[]}`)) })
	if got := c.BudgetUsed(); got != 0 {
		t.Fatalf("fresh budget = %d", got)
	}
	for i := 0; i < 3; i++ {
		if _, err := c.ListVatPcs(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := c.BudgetUsed(); got != 3 {
		t.Fatalf("BudgetUsed = %d, want 3", got)
	}
	if got := c.BudgetUsed(); got != 3 {
		t.Fatalf("reading the budget must not spend it: %d", got)
	}
	if (&Client{}).BudgetUsed() != 0 {
		t.Fatal("an unmetered client reports 0")
	}
}
