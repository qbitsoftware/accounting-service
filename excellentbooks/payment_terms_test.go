package excellentbooks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestListPaymentTerms_ParsesPDVcEnvelope locks in the JSON shape we expect
// from EB's PDVc register. If EB ever changes the envelope, this test fails
// loudly instead of the dropdown silently going empty in production.
func TestListPaymentTerms_ParsesPDVcEnvelope(t *testing.T) {
	// Minimal-but-realistic captured response shape, mirroring how the other
	// register endpoints (ObjVc, PRVc, DepVc) wrap their rows.
	respBody := `{
		"data": {
			"@register": "PDVc",
			"@sequence": "42",
			"@systemversion": "8.5",
			"PDVc": [
				{"Code": "K",  "pdComment": "Sularaha",   "PDType": "2", "pdays": "0",  "Closed": "0"},
				{"Code": "P14","pdComment": "14 päeva",   "PDType": "1", "pdays": "14", "Closed": "0"},
				{"Code": "OLD","pdComment": "Suletud",    "PDType": "1", "pdays": "30", "Closed": "1"},
				{"Code": "",   "pdComment": "Empty code", "PDType": "1", "pdays": "0",  "Closed": "0"}
			]
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/PDVc") {
			t.Errorf("expected request to /PDVc, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respBody))
	}))
	defer srv.Close()

	client := New(Config{BaseURL: srv.URL, CompanyCode: "1", Username: "u", Password: "p"})

	terms, _, err := client.ListPaymentTerms(context.Background(), ListParams{Limit: 1000})
	if err != nil {
		t.Fatalf("ListPaymentTerms: %v", err)
	}
	if len(terms) != 4 {
		t.Fatalf("expected 4 raw rows (filtering happens in the adapter, not the client), got %d", len(terms))
	}

	// Sanity-check the first two rows so we'd notice if the field tags drift.
	if terms[0].Code != "K" || terms[0].Comment != "Sularaha" || terms[0].PDType != "2" {
		t.Errorf("row 0 mis-parsed: %+v", terms[0])
	}
	if terms[1].Code != "P14" || terms[1].NetDays != "14" {
		t.Errorf("row 1 mis-parsed: %+v", terms[1])
	}
}

// TestListPaymentTerms_EmptyRegister_ReturnsEmptySlice — covers the
// scenario the frontend now degrades gracefully on (empty PDVc → no
// dropdown lock, no PayDeal sent, EB uses customer default).
func TestListPaymentTerms_EmptyRegister_ReturnsEmptySlice(t *testing.T) {
	respBody, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"@register": "PDVc",
			"@sequence": "0",
			"PDVc":      []any{},
		},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(respBody)
	}))
	defer srv.Close()

	client := New(Config{BaseURL: srv.URL, Username: "u", Password: "p"})

	terms, _, err := client.ListPaymentTerms(context.Background(), ListParams{})
	if err != nil {
		t.Fatalf("ListPaymentTerms: %v", err)
	}
	if len(terms) != 0 {
		t.Fatalf("expected 0 terms, got %d", len(terms))
	}
}
