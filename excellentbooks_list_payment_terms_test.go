package accounting

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestListPaymentTerms_FiltersClosedAndEmpty exercises the adapter's mapping
// from the raw EB register response to the provider-agnostic []PaymentTerm.
// The dropdown depends on this filtering: closed terms shouldn't appear, and
// empty-code rows would crash Radix Select on the frontend.
func TestListPaymentTerms_FiltersClosedAndEmpty(t *testing.T) {
	respBody := `{
		"data": {
			"@register": "PDVc",
			"PDVc": [
				{"Code": "K",   "pdComment": "Sularaha", "PDType": "2", "pdays": "0",  "Closed": "0"},
				{"Code": "P14", "pdComment": "14 päeva", "PDType": "1", "pdays": "14", "Closed": "0"},
				{"Code": "OLD", "pdComment": "Suletud",  "PDType": "1", "pdays": "30", "Closed": "1"},
				{"Code": "",    "pdComment": "Empty",    "PDType": "1", "pdays": "0",  "Closed": "0"}
			]
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/PDVc") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(respBody))
	}))
	defer srv.Close()

	p := providerWith(srv.URL)

	terms, err := p.ListPaymentTerms(context.Background())
	if err != nil {
		t.Fatalf("ListPaymentTerms: %v", err)
	}

	if len(terms) != 2 {
		t.Fatalf("expected 2 terms after filtering closed+empty, got %d: %+v", len(terms), terms)
	}

	if terms[0].Code != "K" || terms[0].PDType != "2" || terms[0].Label != "Sularaha" {
		t.Errorf("term[0] = %+v, want K/2/Sularaha", terms[0])
	}
	if terms[1].Code != "P14" || terms[1].NetDays != 14 {
		t.Errorf("term[1] = %+v, want P14 with 14 net days", terms[1])
	}
}
