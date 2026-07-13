package simplbooks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecimalUnmarshalTolerance(t *testing.T) {
	cases := map[string]string{
		`13.73`: "13.73",
		`"3.5"`: "3.5", // quoted number
		`null`:  "0",   // null → zero
		`""`:    "0",   // empty string → zero
		`0`:     "0",
	}
	for raw, want := range cases {
		var d Decimal
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			t.Fatalf("Unmarshal(%s): %v", raw, err)
		}
		if d.String() != want {
			t.Errorf("Unmarshal(%s) = %s, want %s", raw, d.String(), want)
		}
	}
}

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := New(Config{BaseURL: srv.URL, CompanyID: "CID", Token: "TOK", HTTPClient: srv.Client()})
	return c, srv.Close
}

func TestListInvoicesEnvelopeAndKey(t *testing.T) {
	// invoices/list wraps each element under the lowercase "invoices" key.
	body := `{
	  "status": 200,
	  "duration": 0.05,
	  "data": [
	    {"invoices": {"id": 1, "number": "18001", "total_sum": 16.48, "vat": 2.75, "created": "2024-01-05", "reference": "201506162801"}},
	    {"invoices": {"id": 2, "number": "18002", "total_sum": "9.90", "created": "2024-02-05"}}
	  ]
	}`
	c, closeFn := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Simplbooks-Token"); got != "TOK" {
			t.Errorf("missing/wrong token header: %q", got)
		}
		if r.URL.Path != "/CID/api/invoices/list" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(body))
	})
	defer closeFn()

	invs, err := c.ListInvoices(context.Background(), ListParams{})
	if err != nil {
		t.Fatalf("ListInvoices: %v", err)
	}
	if len(invs) != 2 {
		t.Fatalf("got %d invoices, want 2", len(invs))
	}
	if invs[0].Number != "18001" {
		t.Errorf("inv0 number = %s, want 18001", invs[0].Number)
	}
	if invs[0].TotalSum.String() != "16.48" {
		t.Errorf("inv0 total = %s, want 16.48", invs[0].TotalSum.String())
	}
	if invs[0].Reference != "201506162801" {
		t.Errorf("inv0 reference = %s", invs[0].Reference)
	}
	// Quoted number must parse too.
	if invs[1].TotalSum.String() != "9.9" {
		t.Errorf("inv1 total = %s, want 9.9", invs[1].TotalSum.String())
	}
}

func TestErrorEnvelopeDetected(t *testing.T) {
	// A 400 with an errors array must surface as an APIError, even though the
	// HTTP body parses as valid JSON.
	c, closeFn := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":400,"duration":0.01,"errors":["Invoice with given ID does not exist."]}`))
	})
	defer closeFn()

	_, err := c.GetInvoice(context.Background(), "999")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", apiErr.StatusCode)
	}
	if len(apiErr.Messages) != 1 {
		t.Fatalf("Messages = %v, want 1 message", apiErr.Messages)
	}
}

func TestCreateReturnsInsertedID(t *testing.T) {
	// inserted_id may come back as a quoted string.
	c, closeFn := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":200,"duration":0.3,"inserted_id":"41","response":"New invoice saved."}`))
	})
	defer closeFn()

	id, err := c.CreateInvoice(context.Background(), map[string]any{"Invoice": map[string]any{}})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if id != 41 {
		t.Errorf("inserted id = %d, want 41", id)
	}
}
