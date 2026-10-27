package merit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// The whole point of splitting is that the slices reconstruct the original
// range exactly: a gap silently drops a day's invoices and an overlap books a
// payment twice.
func TestSplitPeriodTilesRangeExactly(t *testing.T) {
	start, end := day(2024, time.January, 1), day(2026, time.August, 25)
	periods := SplitPeriod(start, end)

	if len(periods) == 0 {
		t.Fatal("expected periods")
	}
	if !periods[0].Start.Equal(start) {
		t.Errorf("first period starts %s, want %s", periods[0].Start, start)
	}
	if last := periods[len(periods)-1]; !last.End.Equal(end) {
		t.Errorf("last period ends %s, want %s", last.End, end)
	}
	for i, p := range periods {
		if p.End.Before(p.Start) {
			t.Errorf("period %d inverted: %s", i, p)
		}
		// +1 because both ends are inclusive.
		if got := int(p.End.Sub(p.Start).Hours()/24) + 1; got > MaxPeriodDays {
			t.Errorf("period %d spans %d days, over the %d cap: %s", i, got, MaxPeriodDays, p)
		}
		if i == 0 {
			continue
		}
		if want := periods[i-1].End.AddDate(0, 0, 1); !p.Start.Equal(want) {
			t.Errorf("period %d starts %s, want %s (gap or overlap)", i, p.Start, want)
		}
	}
}

// Merit counts three *calendar* months, which can be as short as 89 days, so
// the slice width has to clear the shortest one — not the average.
func TestSplitPeriodStaysUnderShortestThreeMonths(t *testing.T) {
	// 1 December → 1 March in a non-leap year is 90 days.
	shortest := day(2027, time.March, 1).Sub(day(2026, time.December, 1)).Hours() / 24
	if MaxPeriodDays >= int(shortest) {
		t.Fatalf("MaxPeriodDays=%d does not clear the shortest 3-month span (%d days)",
			MaxPeriodDays, int(shortest))
	}
}

func TestSplitPeriodEdgeCases(t *testing.T) {
	if got := SplitPeriod(day(2026, time.March, 2), day(2026, time.March, 1)); got != nil {
		t.Errorf("inverted range should yield nothing, got %v", got)
	}
	if got := SplitPeriod(time.Time{}, day(2026, time.March, 1)); got != nil {
		t.Errorf("zero start should yield nothing, got %v", got)
	}
	one := SplitPeriod(day(2026, time.March, 1), day(2026, time.March, 1))
	if len(one) != 1 || !one[0].Start.Equal(one[0].End) {
		t.Errorf("single day should yield one 1-day period, got %v", one)
	}
	short := SplitPeriod(day(2026, time.January, 1), day(2026, time.February, 1))
	if len(short) != 1 {
		t.Errorf("range inside the cap should be one call, got %d", len(short))
	}
}

// stubMerit returns a server that records every period it was asked for and
// answers with handler's result.
func stubMerit(t *testing.T, handler func(w http.ResponseWriter, params map[string]any)) (*Client, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var params map[string]any
		_ = json.NewDecoder(r.Body).Decode(&params)
		mu.Lock()
		seen = append(seen, fmt.Sprintf("%v..%v", params["PeriodStart"], params["PeriodEnd"]))
		mu.Unlock()
		handler(w, params)
	}))
	t.Cleanup(srv.Close)

	c := New(Config{
		APIURL:           srv.URL + "/",
		APIID:            "test-id",
		APIKey:           "test-key",
		DisableRateLimit: true,
	})
	return c, &seen
}

// A year of history is not an error — it is more requests.
func TestListInvoicesRangeSplitsAndMerges(t *testing.T) {
	c, seen := stubMerit(t, func(w http.ResponseWriter, params map[string]any) {
		_ = json.NewEncoder(w).Encode([]InvoiceListItem{
			{SIHId: params["PeriodStart"].(string), InvoiceNo: "INV"},
		})
	})

	got, err := c.ListInvoicesRange(context.Background(),
		day(2025, time.August, 25), day(2026, time.August, 25), ListInvoicesParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*seen) < 4 {
		t.Errorf("a 365-day range should take several calls, took %d: %v", len(*seen), *seen)
	}
	if len(got) != len(*seen) {
		t.Errorf("got %d invoices from %d calls, want one each", len(got), len(*seen))
	}
	for _, s := range *seen {
		if strings.Contains(s, "<nil>") {
			t.Errorf("period not set on call: %s", s)
		}
	}
}

// Other fields of params must reach every chunked call — dropping DateType
// would silently turn a changed-date sweep into a document-date one.
func TestListInvoicesRangeCarriesParamsToEveryCall(t *testing.T) {
	var mu sync.Mutex
	var dateTypes []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var params map[string]any
		_ = json.NewDecoder(r.Body).Decode(&params)
		mu.Lock()
		dateTypes = append(dateTypes, params["DateType"])
		mu.Unlock()
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := New(Config{APIURL: srv.URL + "/", APIID: "i", APIKey: "k", DisableRateLimit: true})
	dt := 1
	_, err := c.ListInvoicesRange(context.Background(),
		day(2026, time.January, 1), day(2026, time.August, 25),
		ListInvoicesParams{DateType: &dt, UnPaid: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dateTypes) < 2 {
		t.Fatalf("expected multiple calls, got %d", len(dateTypes))
	}
	for i, v := range dateTypes {
		if v != float64(1) {
			t.Errorf("call %d lost DateType: got %v", i, v)
		}
	}
}

// One bad window must not cost a club the other eleven.
func TestRangePartialFailureKeepsGoodPeriods(t *testing.T) {
	var n int
	var mu sync.Mutex
	c, _ := stubMerit(t, func(w http.ResponseWriter, params map[string]any) {
		mu.Lock()
		n++
		fail := n == 2
		mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"Message":"Periood liiga pikk(max 3 kuud)"}`))
			return
		}
		_ = json.NewEncoder(w).Encode([]InvoiceListItem{{SIHId: params["PeriodStart"].(string)}})
	})

	got, err := c.ListInvoicesRange(context.Background(),
		day(2025, time.August, 25), day(2026, time.August, 25), ListInvoicesParams{})

	var partial *PartialRangeError
	if !errors.As(err, &partial) {
		t.Fatalf("want *PartialRangeError, got %T: %v", err, err)
	}
	if len(partial.Failed) != 1 {
		t.Errorf("want 1 failed period, got %d", len(partial.Failed))
	}
	if len(got) == 0 {
		t.Error("successful periods were discarded along with the failed one")
	}
	if !IsPeriodTooLong(partial.Failed[0].Err) {
		t.Errorf("period-too-long not recognised: %v", partial.Failed[0].Err)
	}
}

// When nothing succeeded there is no partial result to hand back, and callers
// must not mistake an empty slice for "no invoices in this range".
func TestRangeTotalFailureReturnsNoResults(t *testing.T) {
	c, _ := stubMerit(t, func(w http.ResponseWriter, _ map[string]any) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`nope`))
	})
	got, err := c.ListInvoicesRange(context.Background(),
		day(2025, time.August, 25), day(2026, time.August, 25), ListInvoicesParams{})
	if err == nil {
		t.Fatal("want error")
	}
	if got != nil {
		t.Errorf("want no results on total failure, got %d", len(got))
	}
}

// Boundaries are built not to overlap, but Merit treating a period end as
// inclusive on both sides must not book the same payment twice.
func TestRangeDeduplicates(t *testing.T) {
	c, _ := stubMerit(t, func(w http.ResponseWriter, _ map[string]any) {
		_ = json.NewEncoder(w).Encode([]PaymentListItem{{PIHId: "same"}})
	})
	got, err := c.ListPaymentsRange(context.Background(),
		day(2025, time.August, 25), day(2026, time.August, 25), ListPaymentsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("want 1 payment after dedupe, got %d", len(got))
	}
}

// Merit sends the payment id as PHId; deduplicating on the PIHId it never
// fills kept every copy a window boundary returned twice.
func TestRangeDeduplicatesOnTheIDMeritSends(t *testing.T) {
	c, _ := stubMerit(t, func(w http.ResponseWriter, _ map[string]any) {
		_, _ = w.Write([]byte(`[{"PHId":"same","Amount":30.00}]`))
	})
	got, err := c.ListPaymentsRange(context.Background(),
		day(2025, time.August, 25), day(2026, time.August, 25), ListPaymentsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("want 1 payment after dedupe, got %d", len(got))
	}
}

func TestRetriesRateLimitAndSucceeds(t *testing.T) {
	var n int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		attempt := n
		mu.Unlock()
		if attempt == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := New(Config{APIURL: srv.URL + "/", APIID: "i", APIKey: "k", DisableRateLimit: true})
	if _, err := c.ListTaxes(context.Background()); err != nil {
		t.Fatalf("a rate-limited request should be retried, got %v", err)
	}
	if n != 2 {
		t.Errorf("want 2 attempts, got %d", n)
	}
}

// A 400 is an answer about the request. Retrying it just burns quota that the
// rest of the sync needs.
func TestDoesNotRetryClientErrors(t *testing.T) {
	var n int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		mu.Unlock()
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"Message":"Periood liiga pikk(max 3 kuud)"}`))
	}))
	defer srv.Close()

	c := New(Config{APIURL: srv.URL + "/", APIID: "i", APIKey: "k", DisableRateLimit: true})
	_, err := c.ListTaxes(context.Background())
	if err == nil {
		t.Fatal("want error")
	}
	if n != 1 {
		t.Errorf("want 1 attempt for a 400, got %d", n)
	}
}

func TestRetryAfterParsing(t *testing.T) {
	h := http.Header{}
	if _, ok := retryAfter(h); ok {
		t.Error("absent header should report false")
	}
	h.Set("Retry-After", "30")
	if d, ok := retryAfter(h); !ok || d != 30*time.Second {
		t.Errorf("delta-seconds: got %v %v", d, ok)
	}
	h.Set("Retry-After", "9999")
	if d, _ := retryAfter(h); d != 60*time.Second {
		t.Errorf("want capped at 60s, got %v", d)
	}
	h.Set("Retry-After", http.TimeFormat)
	if _, ok := retryAfter(h); ok {
		t.Error("unparseable header should report false")
	}
	h.Set("Retry-After", time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat))
	if _, ok := retryAfter(h); ok {
		t.Error("past date should report false")
	}
}

// Merit's quota is per API ID, and clients are built per operation — a limiter
// that lives on one Client throttles nothing once two operations overlap.
func TestSharedLimiterPerAPIID(t *testing.T) {
	resetSharedLimiters()
	t.Cleanup(resetSharedLimiters)

	a := New(Config{APIURL: EstoniaURL, APIID: "same", APIKey: "k"})
	b := New(Config{APIURL: EstoniaURL, APIID: "same", APIKey: "k"})
	if a.limiter != b.limiter {
		t.Error("two clients for one API ID must share a limiter")
	}
	c := New(Config{APIURL: EstoniaURL, APIID: "other", APIKey: "k"})
	if a.limiter == c.limiter {
		t.Error("different API IDs must not share a budget")
	}
	d := New(Config{APIURL: PolandURL, APIID: "same", APIKey: "k"})
	if a.limiter == d.limiter {
		t.Error("different regions are separate quotas")
	}
}

// One lax caller must not raise the ceiling for everyone else on the key.
func TestSharedLimiterTakesMostConservative(t *testing.T) {
	resetSharedLimiters()
	t.Cleanup(resetSharedLimiters)

	loose := New(Config{APIURL: EstoniaURL, APIID: "x", APIKey: "k",
		RequestInterval: 10 * time.Millisecond, Burst: 50})
	tight := New(Config{APIURL: EstoniaURL, APIID: "x", APIKey: "k",
		RequestInterval: time.Second, Burst: 1})

	if loose.limiter != tight.limiter {
		t.Fatal("expected a shared limiter")
	}
	if got, want := tight.limiter.Burst(), 1; got != want {
		t.Errorf("burst not tightened: got %d want %d", got, want)
	}
	if tight.limiter.Limit() > 1.1 {
		t.Errorf("rate not tightened: %v", tight.limiter.Limit())
	}
}

func TestDisableRateLimitLeavesNoLimiter(t *testing.T) {
	resetSharedLimiters()
	t.Cleanup(resetSharedLimiters)
	c := New(Config{APIURL: EstoniaURL, APIID: "z", APIKey: "k", DisableRateLimit: true})
	if c.limiter != nil {
		t.Error("DisableRateLimit should leave no limiter")
	}
}

func TestDefaultHTTPClientHasTimeout(t *testing.T) {
	c := New(Config{APIID: "i", APIKey: "k", DisableRateLimit: true})
	if c.httpClient.Timeout == 0 {
		t.Error("default client must not hang forever")
	}
}

// A caller must be able to recognise an incomplete range through the adapter's
// ProviderError wrapper, without importing the merit package.
func TestPartialResultSurvivesWrapping(t *testing.T) {
	inner := &PartialRangeError{
		Failed: []PeriodError{{Period: Period{Start: day(2026, time.January, 1), End: day(2026, time.March, 1)}, Err: errors.New("boom")}},
		Total:  5,
	}
	failed, total := inner.PartialResult()
	if failed != 1 || total != 5 {
		t.Errorf("PartialResult() = %d,%d want 1,5", failed, total)
	}
	if !strings.Contains(inner.Error(), "1 of 5 periods failed") {
		t.Errorf("unhelpful message: %s", inner.Error())
	}
	// Unwrap must reach the underlying cause so errors.Is/As still work for
	// auth failures, which look identical in every period.
	if !errors.Is(errors.Join(inner), inner) {
		t.Error("errors.Is should match the partial error itself")
	}
}
