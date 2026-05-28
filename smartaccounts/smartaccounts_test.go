package smartaccounts

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// --- Signing ---

func TestSignDeterministic(t *testing.T) {
	a := sign("secret", "timestamp=01012025120000&apikey=pub")
	b := sign("secret", "timestamp=01012025120000&apikey=pub")
	if a == "" {
		t.Fatal("signature should not be empty")
	}
	if a != b {
		t.Fatalf("signatures should be deterministic: %q vs %q", a, b)
	}
	if len(a) != 64 {
		t.Errorf("HMAC-SHA256 hex should be 64 chars, got %d", len(a))
	}
}

func TestSignDifferentInputs(t *testing.T) {
	base := sign("secret", "input")
	if sign("other", "input") == base {
		t.Error("different keys should produce different signatures")
	}
	if sign("secret", "input2") == base {
		t.Error("different messages should produce different signatures")
	}
}

func TestTimestampFormat(t *testing.T) {
	ts := timestamp()
	if len(ts) != 14 {
		t.Fatalf("timestamp should be 14 chars (ddMMyyyyHHmmss), got %d: %q", len(ts), ts)
	}
	if _, err := time.Parse("02012006150405", ts); err != nil {
		t.Errorf("timestamp should parse as ddMMyyyyHHmmss: %v", err)
	}
}

// --- Query assembly ---

func TestEncodeQuery(t *testing.T) {
	params := url.Values{}
	params.Set("dateFrom", "01.01.2025")
	q := encodeQuery("pub-key", params)

	if !strings.HasPrefix(q, "timestamp=") {
		t.Errorf("query must start with timestamp: %q", q)
	}
	if !strings.Contains(q, "&apikey=pub-key") {
		t.Errorf("query must contain apikey: %q", q)
	}
	if !strings.Contains(q, "&dateFrom=01.01.2025") {
		t.Errorf("query must contain caller params: %q", q)
	}
	if strings.Contains(q, "signature") {
		t.Errorf("encoded query must NOT contain the signature param (added after signing): %q", q)
	}
}

// --- End-to-end request signing against a mock server ---

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	return New(Config{
		Host:       strings.TrimPrefix(srv.URL, "https://"),
		APIKey:     "pub-key",
		SecretKey:  "secret-key",
		HTTPClient: srv.Client(),
	})
}

func TestRequestSignatureMatches(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.RawQuery
		body, _ := io.ReadAll(r.Body)

		idx := strings.Index(raw, "&signature=")
		if idx < 0 {
			t.Errorf("request must carry a signature param: %q", raw)
			http.Error(w, "no signature", 400)
			return
		}
		signedPart := raw[:idx]
		gotSig := raw[idx+len("&signature="):]

		want := sign("secret-key", signedPart+string(body))
		if gotSig != want {
			t.Errorf("signature mismatch:\n got  %s\n want %s\n signed input: %q + body %q", gotSig, want, signedPart, body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"vatPc":[]}`))
	})

	if _, err := c.ListVatPcs(context.Background()); err != nil {
		t.Fatalf("ListVatPcs: %v", err)
	}
}

func TestPostBodyIsSignedAndSent(t *testing.T) {
	var gotBody string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)

		raw := r.URL.RawQuery
		idx := strings.Index(raw, "&signature=")
		gotSig := raw[idx+len("&signature="):]
		if want := sign("secret-key", raw[:idx]+gotBody); gotSig != want {
			t.Errorf("post signature must cover query+body; got %s want %s", gotSig, want)
		}
		_, _ = w.Write([]byte(`{"invoiceId":"42","invoiceNumber":"2025-1"}`))
	})

	resp, err := c.CreateInvoice(context.Background(), CreateInvoiceRequest{
		ClientID: "7",
		Date:     "01.01.2025",
		Rows:     []InvoiceRowInput{{Code: "00010", Quantity: dec("1"), Price: dec("10")}},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if resp.InvoiceID != "42" {
		t.Errorf("expected invoiceId 42, got %q", resp.InvoiceID)
	}
	if strings.ContainsAny(gotBody, "\n\r") {
		t.Errorf("request body must not contain line breaks: %q", gotBody)
	}
	if !strings.Contains(gotBody, `"clientId":"7"`) {
		t.Errorf("body should carry clientId: %q", gotBody)
	}
}

// --- Pagination + list extraction ---

func TestListPaginationFollowsPages(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("pageNumber") {
		case "1":
			_, _ = w.Write([]byte(`{"clientInvoices":[{"id":"1"},{"id":"2"}],"hasMoreEntries":true,"deleted":["99"]}`))
		case "2":
			_, _ = w.Write([]byte(`{"clientInvoices":[{"id":"3"}],"hasMoreEntries":false}`))
		default:
			t.Errorf("unexpected pageNumber %q", r.URL.Query().Get("pageNumber"))
		}
	})

	items, deleted, err := c.ListInvoices(context.Background(), ListInvoicesParams{DateFrom: "01.01.2025"})
	if err != nil {
		t.Fatalf("ListInvoices: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 invoices across 2 pages, got %d", len(items))
	}
	if len(deleted) != 1 || deleted[0] != "99" {
		t.Errorf("expected deleted=[99] from page 1, got %v", deleted)
	}
}

func TestExtractList(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"named array", `{"clientInvoices":[{"id":"1"},{"id":"2"}],"hasMoreEntries":false}`, 2},
		{"bare array", `[{"id":"1"}]`, 1},
		{"deleted only", `{"deleted":["1","2"],"hasMoreEntries":false}`, 0},
		{"empty object", `{"hasMoreEntries":false}`, 0},
		{"deleted alongside data ignored", `{"clientInvoices":[{"id":"1"}],"deleted":["9"],"hasMoreEntries":false}`, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arr, err := extractList(json.RawMessage(tt.in))
			if err != nil {
				t.Fatalf("extractList: %v", err)
			}
			var items []json.RawMessage
			if err := json.Unmarshal(arr, &items); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(items) != tt.want {
				t.Errorf("got %d items, want %d", len(items), tt.want)
			}
		})
	}
}

func TestExtractListAmbiguousErrors(t *testing.T) {
	// Two non-deleted arrays: must error rather than non-deterministically
	// picking one (Go map iteration is randomized).
	_, err := extractList(json.RawMessage(`{"clientInvoices":[{"id":"1"}],"offers":[{"id":"2"}]}`))
	if err == nil {
		t.Fatal("expected an error for an ambiguous envelope with multiple arrays")
	}
}

func TestFlexStringAcceptsBothForms(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"json string", `"42"`, "42"},
		{"json number", `42`, "42"},
		{"json null", `null`, ""},
		{"large number", `1234567890`, "1234567890"},
		{"alphanumeric string", `"INV-001"`, "INV-001"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var s flexString
			if err := json.Unmarshal([]byte(c.in), &s); err != nil {
				t.Fatalf("unmarshal %q: %v", c.in, err)
			}
			if string(s) != c.want {
				t.Errorf("got %q, want %q", string(s), c.want)
			}
		})
	}
}

func TestEUSummerTime(t *testing.T) {
	cases := []struct {
		name string
		utc  time.Time
		want bool
	}{
		{"midwinter", time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), false},
		{"midsummer", time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), true},
		// 2026 DST: starts 29 Mar 01:00 UTC, ends 25 Oct 01:00 UTC.
		{"just before spring switch", time.Date(2026, 3, 29, 0, 59, 0, 0, time.UTC), false},
		{"just after spring switch", time.Date(2026, 3, 29, 1, 1, 0, 0, time.UTC), true},
		{"just before autumn switch", time.Date(2026, 10, 25, 0, 59, 0, 0, time.UTC), true},
		{"just after autumn switch", time.Date(2026, 10, 25, 1, 1, 0, 0, time.UTC), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isEUSummerTime(c.utc); got != c.want {
				t.Errorf("isEUSummerTime(%s) = %v, want %v", c.utc, got, c.want)
			}
		})
	}
}

func TestAPIErrorOnNon2xx(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad signature"}`, http.StatusUnauthorized)
	})
	_, err := c.ListVatPcs(context.Background())
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", apiErr.StatusCode)
	}
}
