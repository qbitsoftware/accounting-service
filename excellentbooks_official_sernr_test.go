package accounting

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

// The caller's invoice number must reach EB as OfficialSerNr — EB assigns its
// own SerNr, so without it the document cannot be found by the number the
// member was sent.
func TestCreateInvoice_EmitsOfficialSerNr(t *testing.T) {
	captured, srv := captureFormBody(t)
	defer srv.Close()
	p := providerWith(srv.URL)

	_, err := p.CreateInvoice(context.Background(), CreateInvoiceInput{
		CustomerID: "C1",
		InvoiceNo:  "26090012",
		OfficialNo: "26090012",
		Lines: []CreateInvoiceLineInput{
			{Code: "TUITION", Quantity: decimal.NewFromInt(1), UnitPrice: decimal.NewFromInt(50)},
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if got := parseFormBody(t, *captured).Get("set_field.OfficialSerNr"); got != "26090012" {
		t.Errorf("OfficialSerNr = %q, want 26090012", got)
	}
}

func TestCreateCreditNote_EmitsOfficialSerNr(t *testing.T) {
	captured, srv := captureFormBody(t)
	defer srv.Close()
	p := providerWith(srv.URL)

	_, err := p.CreateCreditNote(context.Background(), CreateCreditNoteInput{
		CustomerID:        "C1",
		InvoiceNo:         "KR-26090012",
		OfficialNo:        "KR-26090012",
		OriginalInvoiceNo: "200000",
		Lines: []CreateInvoiceLineInput{
			{Code: "TUITION", Quantity: decimal.NewFromInt(-1), UnitPrice: decimal.NewFromInt(50)},
		},
	})
	if err != nil {
		t.Fatalf("CreateCreditNote: %v", err)
	}
	if got := parseFormBody(t, *captured).Get("set_field.OfficialSerNr"); got != "KR-26090012" {
		t.Errorf("OfficialSerNr = %q, want KR-26090012", got)
	}
}

// Opt-in: an invoice number alone must not reach OfficialSerNr, or every
// club on EB would start writing into a field their accountant may number
// themselves.
func TestCreateInvoice_OmitsOfficialSerNrWhenNotAsked(t *testing.T) {
	captured, srv := captureFormBody(t)
	defer srv.Close()
	p := providerWith(srv.URL)

	_, err := p.CreateInvoice(context.Background(), CreateInvoiceInput{
		CustomerID: "C1",
		InvoiceNo:  "26090012",
		Lines: []CreateInvoiceLineInput{
			{Code: "TUITION", Quantity: decimal.NewFromInt(1), UnitPrice: decimal.NewFromInt(50)},
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if _, ok := parseFormBody(t, *captured)["set_field.OfficialSerNr"]; ok {
		t.Error("OfficialSerNr sent without OfficialNo")
	}
}
