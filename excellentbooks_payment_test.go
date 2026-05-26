package accounting

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// receiptCreatedResponse is the minimum IPVc payload that lets the adapter parse
// a created receipt. Field values are irrelevant — these are emit-shape tests.
const receiptCreatedResponse = `{
	"data": {
		"@register": "IPVc",
		"IPVc": [{"SerNr": "300001"}]
	}
}`

// captureReceiptBody records the form-encoded POST sent to the EB receipt
// register (IPVc) and replies with a stub receipt.
func captureReceiptBody(t *testing.T) (*string, *httptest.Server) {
	t.Helper()
	captured := new(string)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/IPVc") {
			t.Errorf("unexpected path %s, want .../IPVc", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		*captured = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(receiptCreatedResponse))
	}))
	return captured, srv
}

// --- Bug-prevention: PayMode on outbound receipts ---
//
// This is the test that would have caught the "EB payment push uses empty
// PayMode → silently dropped" bug. EB requires a PayMode on every receipt, and
// it must be carried in CreatePaymentInput.BankID.
func TestCreatePayment_EmitsPayMode(t *testing.T) {
	captured, srv := captureReceiptBody(t)
	defer srv.Close()
	p := providerWith(srv.URL)

	err := p.CreatePayment(context.Background(), CreatePaymentInput{
		CustomerCode: "C1",
		InvoiceNo:    "260123",
		Amount:       decimal.NewFromInt(40),
		Currency:     "EUR",
		BankID:       "P2", // the EB PayMode code
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	form := parseFormBody(t, *captured)

	// THE guard — the PayMode must reach EB.
	if got := form.Get("set_field.PayMode"); got != "P2" {
		t.Errorf("set_field.PayMode = %q, want P2 — receipts must carry a PayMode", got)
	}
	if got := form.Get("set_row_field.0.InvoiceNr"); got != "260123" {
		t.Errorf("row InvoiceNr = %q, want 260123", got)
	}
	if got := form.Get("set_row_field.0.CustCode"); got != "C1" {
		t.Errorf("row CustCode = %q, want C1", got)
	}
	if got := form.Get("set_row_field.0.RecVal"); got != "40" {
		t.Errorf("row RecVal = %q, want 40", got)
	}
	if got := form.Get("set_row_field.0.stp"); got != "1" {
		t.Errorf("row stp = %q, want 1 (normal receipt row)", got)
	}
	if got := form.Get("set_field.OKFlag"); got != "1" {
		t.Errorf("OKFlag = %q, want 1 (confirmed)", got)
	}
}

// CreatePayment must refuse an empty PayMode locally, with a clear error, rather
// than letting EB reject it server-side — and it must not hit the API at all.
func TestCreatePayment_RejectsEmptyPayMode(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	p := providerWith(srv.URL)

	err := p.CreatePayment(context.Background(), CreatePaymentInput{
		CustomerCode: "C1",
		InvoiceNo:    "260123",
		Amount:       decimal.NewFromInt(40),
		// BankID intentionally empty.
	})
	if err == nil {
		t.Fatal("expected an error for empty PayMode, got nil")
	}
	if !strings.Contains(err.Error(), "PayMode") {
		t.Errorf("error %q should mention PayMode", err.Error())
	}
	if called {
		t.Error("CreatePayment hit the EB API despite empty PayMode — must fail before the request")
	}
}

// --- Bug-prevention: ArtCode on credit-note rows ---
//
// EB rejects a row with an empty ArtCode ("Täitmata kanded ei ole lubatud").
// The adapter must carry line.Code through to set_row_field.N.ArtCode so the
// backend's resolved article code actually reaches EB.
func TestCreateCreditNote_EmitsArtCodePerRow(t *testing.T) {
	captured, srv := captureFormBody(t)
	defer srv.Close()
	p := providerWith(srv.URL)

	_, err := p.CreateCreditNote(context.Background(), CreateCreditNoteInput{
		CustomerID:        "C1",
		OriginalInvoiceNo: "200000",
		Lines: []CreateInvoiceLineInput{
			{
				Code:        "ÕPPEMAKS",
				Quantity:    decimal.NewFromInt(1),
				UnitPrice:   decimal.NewFromInt(-99),
				AccountCode: "3702",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateCreditNote: %v", err)
	}

	form := parseFormBody(t, *captured)
	if got := form.Get("set_row_field.0.ArtCode"); got != "ÕPPEMAKS" {
		t.Errorf("row 0 ArtCode = %q, want ÕPPEMAKS — credit-note rows must carry the article code", got)
	}
}

func TestCreateInvoice_EmitsArtCodePerRow(t *testing.T) {
	captured, srv := captureFormBody(t)
	defer srv.Close()
	p := providerWith(srv.URL)

	_, err := p.CreateInvoice(context.Background(), CreateInvoiceInput{
		CustomerID: "C1",
		Lines: []CreateInvoiceLineInput{{
			Code:        "ÕPPEMAKS",
			Quantity:    decimal.NewFromInt(1),
			UnitPrice:   decimal.NewFromInt(99),
			AccountCode: "3702",
		}},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	form := parseFormBody(t, *captured)
	if got := form.Get("set_row_field.0.ArtCode"); got != "ÕPPEMAKS" {
		t.Errorf("row 0 ArtCode = %q, want ÕPPEMAKS", got)
	}
}
