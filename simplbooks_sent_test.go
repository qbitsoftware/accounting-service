package accounting

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// sentCaptureServer stands in for SimplBooks: it records the create body and the
// paths hit, and answers every call with a plausible envelope.
func sentCaptureServer(t *testing.T, body *map[string]any, paths *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.URL.Path)
		switch {
		case strings.Contains(r.URL.Path, "invoices/create"):
			raw, _ := io.ReadAll(r.Body)
			var doc struct {
				Invoice map[string]any `json:"Invoice"`
			}
			json.Unmarshal(raw, &doc)
			*body = doc.Invoice
			w.Write([]byte(`{"status":200,"inserted_id":11,"response":"OK"}`))
		case strings.Contains(r.URL.Path, "invoices/get/"):
			json.NewEncoder(w).Encode(map[string]any{
				"status": 200,
				"data":   map[string]any{"Invoice": map[string]any{"id": 11}, "Task": []any{}},
			})
		default:
			w.Write([]byte(`{"status":200,"response":"OK"}`))
		}
	}))
}

func newSentTestProvider(t *testing.T, srv *httptest.Server) *simplbooksProvider {
	t.Helper()
	return newSimplBooksProvider(Config{
		APIID: "co", APIKey: "tok",
		Extra: map[string]string{"base_url": srv.URL},
	})
}

// An invoice klubio has already mailed out must not land in SimplBooks looking
// unsent, so SentDate has to reach the create body as `sent`.
func TestCreateInvoiceCarriesSentDate(t *testing.T) {
	var body map[string]any
	var paths []string
	srv := sentCaptureServer(t, &body, &paths)
	defer srv.Close()

	p := newSentTestProvider(t, srv)
	_, err := p.CreateInvoice(context.Background(), CreateInvoiceInput{
		CustomerID: "5",
		DocDate:    time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		SentDate:   time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if body["sent"] != "2026-07-02" {
		t.Errorf("sent = %v, want 2026-07-02", body["sent"])
	}
}

// A zero SentDate must omit the key entirely: SimplBooks stores an empty string
// as the 0000-00-00 sentinel rather than treating it as "not set".
func TestCreateInvoiceOmitsUnsetSentDate(t *testing.T) {
	var body map[string]any
	var paths []string
	srv := sentCaptureServer(t, &body, &paths)
	defer srv.Close()

	p := newSentTestProvider(t, srv)
	if _, err := p.CreateInvoice(context.Background(), CreateInvoiceInput{CustomerID: "5"}); err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if _, ok := body["sent"]; ok {
		t.Errorf("sent must be absent when SentDate is zero, got %v", body["sent"])
	}
}

func TestCreateCreditNoteCarriesSentDate(t *testing.T) {
	var body map[string]any
	var paths []string
	srv := sentCaptureServer(t, &body, &paths)
	defer srv.Close()

	p := newSentTestProvider(t, srv)
	_, err := p.CreateCreditNote(context.Background(), CreateCreditNoteInput{
		CustomerID: "5",
		SentDate:   time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateCreditNote: %v", err)
	}
	if body["sent"] != "2026-07-02" {
		t.Errorf("sent = %v, want 2026-07-02", body["sent"])
	}
}

// If the account drops `sent` on create, the invoice would silently display as
// unsent — so the read-back must notice and repair it via invoices/sent.
func TestCreateInvoiceRepairsIgnoredSentDate(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch {
		case strings.Contains(r.URL.Path, "invoices/create"):
			w.Write([]byte(`{"status":200,"inserted_id":11,"response":"OK"}`))
		case strings.Contains(r.URL.Path, "invoices/get/"):
			// The create silently ignored `sent`: 0000-00-00 is SimplBooks' unset sentinel.
			json.NewEncoder(w).Encode(map[string]any{
				"status": 200,
				"data": map[string]any{
					"Invoice": map[string]any{"id": 11, "sent": "0000-00-00"},
					"Task":    []any{},
				},
			})
		default:
			w.Write([]byte(`{"status":200,"response":"OK"}`))
		}
	}))
	defer srv.Close()

	p := newSentTestProvider(t, srv)
	_, err := p.CreateInvoice(context.Background(), CreateInvoiceInput{
		CustomerID: "5",
		SentDate:   time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	var marked bool
	for _, path := range paths {
		if strings.HasSuffix(path, "/invoices/sent/11") {
			marked = true
		}
	}
	if !marked {
		t.Errorf("expected a repair call to invoices/sent/11, got %v", paths)
	}
}

// The repair must not fire when the create honoured `sent` — it would burn a
// request against the 60 req/min budget on every single invoice of a batch.
func TestCreateInvoiceSkipsRepairWhenSentStuck(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch {
		case strings.Contains(r.URL.Path, "invoices/create"):
			w.Write([]byte(`{"status":200,"inserted_id":11,"response":"OK"}`))
		case strings.Contains(r.URL.Path, "invoices/get/"):
			json.NewEncoder(w).Encode(map[string]any{
				"status": 200,
				"data": map[string]any{
					"Invoice": map[string]any{"id": 11, "sent": "2026-07-02"},
					"Task":    []any{},
				},
			})
		default:
			w.Write([]byte(`{"status":200,"response":"OK"}`))
		}
	}))
	defer srv.Close()

	p := newSentTestProvider(t, srv)
	if _, err := p.CreateInvoice(context.Background(), CreateInvoiceInput{
		CustomerID: "5",
		SentDate:   time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	for _, path := range paths {
		if strings.Contains(path, "invoices/sent/") {
			t.Errorf("must not call invoices/sent when the date stuck, got %v", paths)
		}
	}
}

// A create whose sent flag cannot be repaired must still succeed: a retry would
// duplicate the document in the accounting system.
func TestCreateInvoiceSurvivesFailedSentRepair(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "invoices/create"):
			w.Write([]byte(`{"status":200,"inserted_id":11,"response":"OK"}`))
		case strings.Contains(r.URL.Path, "invoices/get/"):
			json.NewEncoder(w).Encode(map[string]any{
				"status": 200,
				"data": map[string]any{
					"Invoice": map[string]any{"id": 11, "sent": "0000-00-00"},
					"Task":    []any{},
				},
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"status":400,"messages":["nope"]}`))
		}
	}))
	defer srv.Close()

	p := newSentTestProvider(t, srv)
	inv, err := p.CreateInvoice(context.Background(), CreateInvoiceInput{
		CustomerID: "5",
		SentDate:   time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateInvoice must not fail on an unmarkable sent flag: %v", err)
	}
	if inv.ID != "11" {
		t.Errorf("invoice id = %q, want 11", inv.ID)
	}
}

func TestMarkInvoiceSentHitsSentEndpoint(t *testing.T) {
	var body map[string]any
	var paths []string
	srv := sentCaptureServer(t, &body, &paths)
	defer srv.Close()

	p := newSentTestProvider(t, srv)
	if err := p.MarkInvoiceSent(context.Background(), "11"); err != nil {
		t.Fatalf("MarkInvoiceSent: %v", err)
	}
	if len(paths) != 1 || !strings.HasSuffix(paths[0], "/invoices/sent/11") {
		t.Errorf("paths = %v, want a single .../invoices/sent/11", paths)
	}
}

// The capability is discovered by type assertion, so SimplBooks must satisfy
// InvoiceSentMarker and the providers with no sent flag must not.
func TestInvoiceSentMarkerCapability(t *testing.T) {
	if _, ok := any(&simplbooksProvider{}).(InvoiceSentMarker); !ok {
		t.Error("simplbooksProvider must implement InvoiceSentMarker")
	}
	if _, ok := any(&meritProvider{}).(InvoiceSentMarker); ok {
		t.Error("meritProvider must not implement InvoiceSentMarker")
	}
}
