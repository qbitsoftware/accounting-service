package accounting

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/qbitsoftware/accounting-service/excellentbooks"
	"github.com/shopspring/decimal"
)

// invoiceCreatedResponse is the minimum payload EB returns from POST /IVVc
// that lets the adapter parse a result. Everything else (account numbers,
// dates, etc.) is irrelevant for these emit-shape tests.
const invoiceCreatedResponse = `{
	"data": {
		"@register": "IVVc",
		"IVVc": [{"SerNr": "200001", "InvDate": "2026-05-08", "CustCode": "C1"}]
	}
}`

// captureFormBody spins up a test server that records the form-encoded POST
// body sent to the EB invoice register and replies with a stub Invoice.
// Returns the recorded body (after the request) and the server.
func captureFormBody(t *testing.T) (*string, *httptest.Server) {
	t.Helper()
	captured := new(string)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/IVVc") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		*captured = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(invoiceCreatedResponse))
	}))
	return captured, srv
}

// providerWith builds an excellentProvider pointed at the given server URL.
// Lives in the accounting package so it can construct the unexported type
// directly — same trick the merit_test does in its own package.
func providerWith(srvURL string) *excellentProvider {
	return &excellentProvider{
		client: excellentbooks.New(excellentbooks.Config{
			BaseURL:     srvURL,
			CompanyCode: "1",
			Username:    "u",
			Password:    "p",
		}),
	}
}

// parseFormBody decodes EB's "+ -> %20" form encoding back into a flat map
// for assertions.
func parseFormBody(t *testing.T, body string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(body)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	return v
}

// --- Bug-prevention: SalesAcc on credit-note rows ---
//
// This is the test that would have caught the original "kreeditarve hits
// account 7920 instead of 3702" bug. If anyone deletes the SalesAcc emit in
// CreateCreditNote again, this fails.
func TestCreateCreditNote_EmitsSalesAccPerRow(t *testing.T) {
	captured, srv := captureFormBody(t)
	defer srv.Close()
	p := providerWith(srv.URL)

	// Inputs use the credit-note convention merit_sync actually sends: negated
	// quantity (-1) and the ORIGINAL line price (positive for a normal sale line,
	// negative for a discount/manual-adjustment line).
	_, err := p.CreateCreditNote(context.Background(), CreateCreditNoteInput{
		CustomerID:        "C1",
		InvoiceNo:         "KRARVE000001",
		OriginalInvoiceNo: "200000",
		Currency:          "EUR",
		Lines: []CreateInvoiceLineInput{
			{
				Code:        "TUITION",
				Quantity:    decimal.NewFromInt(-1),
				UnitPrice:   decimal.NewFromInt(99),
				TaxID:       "0",
				AccountCode: "3702",
				Description: "Õppemaks aprill",
			},
			{
				Code:        "FEE",
				Quantity:    decimal.NewFromInt(-1),
				UnitPrice:   decimal.NewFromInt(-10), // discount/manual-adjustment line keeps its negative price
				AccountCode: "3705",
				Description: "Käsitsi muudatus",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateCreditNote: %v", err)
	}

	form := parseFormBody(t, *captured)

	// Header fields.
	if form.Get("set_field.InvType") != "3" {
		t.Errorf("expected InvType=3, got %q", form.Get("set_field.InvType"))
	}
	if form.Get("set_field.CredMark") != "1" {
		t.Errorf("expected CredMark=1, got %q", form.Get("set_field.CredMark"))
	}
	if form.Get("set_field.CredInv") != "200000" {
		t.Errorf("CredInv missing or wrong: %q", form.Get("set_field.CredInv"))
	}

	// THE link-row guard — a single leading stp=3 row carries ONLY OrdRow =
	// credited SerNr (no article). This is how real EB credit invoices link to
	// their original (cmd/eb-credit-dump). Without it EB rejects with 1030
	// "Täitmata kanded" / 1119 "Sisesta krediteeritava arve number".
	if got := form.Get("set_row_field.0.stp"); got != "3" {
		t.Errorf("row 0 stp = %q, want 3 (link row)", got)
	}
	if got := form.Get("set_row_field.0.OrdRow"); got != "200000" {
		t.Errorf("row 0 OrdRow = %q, want 200000 (credited invoice SerNr)", got)
	}
	if got := form.Get("set_row_field.0.ArtCode"); got != "" {
		t.Errorf("row 0 ArtCode = %q, want empty — link row carries no article", got)
	}

	// THE multi-row guard — article rows are stp=1 (normal) so EB does not read
	// each as its own credit link (that triggered 22049 "Erinevate arvete
	// krediteerimine pole lubatud" when stp=3+OrdRow was on every row).
	if got := form.Get("set_row_field.1.stp"); got != "1" {
		t.Errorf("row 1 stp = %q, want 1 (normal article row)", got)
	}
	if got := form.Get("set_row_field.2.stp"); got != "1" {
		t.Errorf("row 2 stp = %q, want 1 (normal article row)", got)
	}
	if got := form.Get("set_row_field.1.OrdRow"); got != "" {
		t.Errorf("row 1 OrdRow = %q, want empty — only the link row carries OrdRow", got)
	}

	// Article rows mirror the ORIGINAL lines: positive quantity, original price.
	if got := form.Get("set_row_field.1.ArtCode"); got != "TUITION" {
		t.Errorf("row 1 ArtCode = %q, want TUITION", got)
	}
	if got := form.Get("set_row_field.1.Quant"); got != "1" {
		t.Errorf("row 1 Quant = %q, want 1 (original positive qty)", got)
	}
	if got := form.Get("set_row_field.1.Price"); got != "99" {
		t.Errorf("row 1 Price = %q, want 99 (original line price)", got)
	}
	if got := form.Get("set_row_field.2.Price"); got != "-10" {
		t.Errorf("row 2 Price = %q, want -10 (discount line keeps its sign)", got)
	}

	// THE bug guard — both article rows must carry SalesAcc.
	if got := form.Get("set_row_field.1.SalesAcc"); got != "3702" {
		t.Errorf("row 1 SalesAcc = %q, want 3702 — credit notes must reverse the same account", got)
	}
	if got := form.Get("set_row_field.2.SalesAcc"); got != "3705" {
		t.Errorf("row 2 SalesAcc = %q, want 3705", got)
	}

	// Spec (description) must also be carried — same forgotten-field family.
	if got := form.Get("set_row_field.1.Spec"); got != "Õppemaks aprill" {
		t.Errorf("row 1 Spec = %q, want Õppemaks aprill", got)
	}
}

// --- Feature: PayDeal forwarded only when set ---

func TestCreateCreditNote_EmitsPayDealWhenSet(t *testing.T) {
	captured, srv := captureFormBody(t)
	defer srv.Close()
	p := providerWith(srv.URL)

	_, err := p.CreateCreditNote(context.Background(), CreateCreditNoteInput{
		CustomerID:      "C1",
		PaymentTermCode: "K",
		Lines: []CreateInvoiceLineInput{{
			Code: "X", Quantity: decimal.NewFromInt(1), UnitPrice: decimal.NewFromInt(-1),
		}},
	})
	if err != nil {
		t.Fatalf("CreateCreditNote: %v", err)
	}

	form := parseFormBody(t, *captured)
	if got := form.Get("set_field.PayDeal"); got != "K" {
		t.Errorf("PayDeal = %q, want K", got)
	}
}

func TestCreateCreditNote_OmitsPayDealWhenUnset(t *testing.T) {
	captured, srv := captureFormBody(t)
	defer srv.Close()
	p := providerWith(srv.URL)

	_, err := p.CreateCreditNote(context.Background(), CreateCreditNoteInput{
		CustomerID: "C1",
		// PaymentTermCode intentionally empty — adapter must NOT emit
		// PayDeal so EB falls back to the customer's default term.
		Lines: []CreateInvoiceLineInput{{
			Code: "X", Quantity: decimal.NewFromInt(1), UnitPrice: decimal.NewFromInt(-1),
		}},
	})
	if err != nil {
		t.Fatalf("CreateCreditNote: %v", err)
	}

	form := parseFormBody(t, *captured)
	if _, present := form["set_field.PayDeal"]; present {
		t.Errorf("PayDeal must be absent when PaymentTermCode is empty, got %q", form.Get("set_field.PayDeal"))
	}
}

// --- Regression guard: regular invoice still emits SalesAcc per row ---
//
// CreateInvoice has had this for a while, but the same copy-paste path that
// dropped it from CreateCreditNote could happen again. Lock it in.
func TestCreateInvoice_EmitsSalesAccPerRow(t *testing.T) {
	captured, srv := captureFormBody(t)
	defer srv.Close()
	p := providerWith(srv.URL)

	_, err := p.CreateInvoice(context.Background(), CreateInvoiceInput{
		CustomerID: "C1",
		Lines: []CreateInvoiceLineInput{{
			Code:        "TUITION",
			Quantity:    decimal.NewFromInt(1),
			UnitPrice:   decimal.NewFromInt(99),
			AccountCode: "3702",
			Description: "Õppemaks",
		}},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	form := parseFormBody(t, *captured)
	if got := form.Get("set_row_field.0.SalesAcc"); got != "3702" {
		t.Errorf("row 0 SalesAcc = %q, want 3702", got)
	}
}
