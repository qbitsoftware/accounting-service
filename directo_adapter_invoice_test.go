package accounting

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// captureDirecto stands in for Directo's XML Direct endpoint and hands back the
// xmldata form field the adapter posted, so a test can assert on the exact
// document that would have gone over the wire.
func captureDirecto(t *testing.T) (*directoProvider, *string) {
	t.Helper()

	var captured string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		captured = r.PostFormValue("xmldata")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<results><result type="0" desc="OK"/></results>`))
	}))
	t.Cleanup(ts.Close)

	p, err := newDirectoProvider(Config{
		APIID:      "test_company",
		APIKey:     "test_token",
		Extra:      map[string]string{"xml_base_url": ts.URL},
		HTTPClient: ts.Client(),
	})
	if err != nil {
		t.Fatalf("newDirectoProvider: %v", err)
	}
	return p, &captured
}

func baseInvoiceInput() CreateInvoiceInput {
	return CreateInvoiceInput{
		InvoiceNo:     "100001",
		CustomerID:    "CUST1",
		CustomerName:  "Acme GmbH",
		CustomerRegNo: "HRB 123456",
		CustomerVATNo: "DE812526315",
		CustomerEmail: "billing@acme.de",
		CustomerType:  "0",
		DocDate:       time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC),
		DueDate:       time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC),
		Currency:      "EUR",
		Lines: []CreateInvoiceLineInput{{
			Code:      "MEMBERSHIP",
			Quantity:  decimal.NewFromInt(1),
			UnitPrice: decimal.NewFromFloat(80.65),
			TaxID:     "24",
		}},
	}
}

// A foreign customer's country must reach Directo's `country` attribute
// (sql asumaa, UI "Sihtriik"). Before this the caller hardcoded "EE" and a
// German company was filed on the ledger as an Estonian one.
func TestDirectoCreateInvoiceStampsCustomerCountry(t *testing.T) {
	p, captured := captureDirecto(t)

	in := baseInvoiceInput()
	in.CustomerCountryCode = "DE"

	if _, err := p.CreateInvoice(context.Background(), in); err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if !strings.Contains(*captured, `country="DE"`) {
		t.Errorf("invoice XML missing country=\"DE\":\n%s", *captured)
	}
	if !strings.Contains(*captured, `vatregno="DE812526315"`) {
		t.Errorf("invoice XML missing the VAT number:\n%s", *captured)
	}
	if !strings.Contains(*captured, `customer_regno="HRB 123456"`) {
		t.Errorf("invoice XML missing the foreign registration code:\n%s", *captured)
	}
}

// vatzone is Directo's KM maa. Absent means "use the customer card's zone,
// else local" — which is what every document sent before the field existed,
// so an unset VATZone must not put the attribute on the wire at all.
func TestDirectoCreateInvoiceOmitsVATZoneWhenUnset(t *testing.T) {
	p, captured := captureDirecto(t)

	if _, err := p.CreateInvoice(context.Background(), baseInvoiceInput()); err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if strings.Contains(*captured, "vatzone") {
		t.Errorf("unset VATZone still emitted the attribute:\n%s", *captured)
	}
}

func TestDirectoCreateInvoiceSendsVATZoneWhenSet(t *testing.T) {
	for _, zone := range []string{"0", "1", "2"} {
		p, captured := captureDirecto(t)

		in := baseInvoiceInput()
		in.VATZone = zone

		if _, err := p.CreateInvoice(context.Background(), in); err != nil {
			t.Fatalf("CreateInvoice: %v", err)
		}
		if want := `vatzone="` + zone + `"`; !strings.Contains(*captured, want) {
			t.Errorf("invoice XML missing %s:\n%s", want, *captured)
		}
	}
}

// A credit note must carry the same country and zone as the invoice it
// reverses, or the two documents land in different VAT declarations.
func TestDirectoCreateCreditNoteMirrorsCountryAndZone(t *testing.T) {
	p, captured := captureDirecto(t)

	_, err := p.CreateCreditNote(context.Background(), CreateCreditNoteInput{
		InvoiceNo:           "100002",
		CustomerID:          "CUST1",
		CustomerName:        "Acme GmbH",
		CustomerCountryCode: "DE",
		VATZone:             "1",
		DocDate:             time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC),
		DueDate:             time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC),
		Currency:            "EUR",
		Lines: []CreateInvoiceLineInput{{
			Code:      "MEMBERSHIP",
			Quantity:  decimal.NewFromInt(1),
			UnitPrice: decimal.NewFromFloat(80.65),
			TaxID:     "24",
		}},
	})
	if err != nil {
		t.Fatalf("CreateCreditNote: %v", err)
	}
	if !strings.Contains(*captured, `country="DE"`) {
		t.Errorf("credit note XML missing country=\"DE\":\n%s", *captured)
	}
	if !strings.Contains(*captured, `vatzone="1"`) {
		t.Errorf("credit note XML missing vatzone=\"1\":\n%s", *captured)
	}
}
