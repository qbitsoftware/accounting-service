package smartaccounts

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

var updateTypeSnapshot = flag.Bool("update-type-snapshot", false, "rewrite testdata/request_types.golden")

// TestRequestTypesUnchanged pins the field names, Go types and JSON tags of
// every type that is marshalled onto SmartAccounts' wire, plus the existing
// decode targets. Receipt-level work adds NEW types beside these; it must never
// touch one of them, because every SmartAccounts club — moved onto the new
// behaviour or not — sends and decodes through exactly these structs.
//
// ListPaymentsParams is deliberately not here: it gains opt-in fields, and what
// it puts on the wire when they are zero is pinned by its own values() test.
func TestRequestTypesUnchanged(t *testing.T) {
	types := []any{
		// requests
		CreatePaymentRequest{}, PaymentPrepayment{}, PaymentRow{},
		CreateInvoiceRequest{}, InvoiceRowInput{},
		CreateClientRequest{}, Address{}, Contact{},
		CreateVendorRequest{}, CreateVendorInvoiceRequest{},
		ArticleItem{}, ListInvoicesParams{},
		// existing decode paths
		page{}, PaymentItem{}, PartnerRef{}, PaymentResponse{},
		InvoiceItem{}, InvoiceRow{}, InvoiceResponse{},
		ClientItem{}, ClientResponse{}, BankAccountItem{}, AccountItem{},
	}
	var b strings.Builder
	for _, v := range types {
		rt := reflect.TypeOf(v)
		fmt.Fprintf(&b, "%s\n", rt.Name())
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			fmt.Fprintf(&b, "\t%s %s `%s`\n", f.Name, f.Type, f.Tag)
		}
	}
	got := b.String()
	const path = "testdata/request_types.golden"
	if *updateTypeSnapshot {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if got != string(want) {
		t.Fatalf("a SmartAccounts wire type changed; existing clubs' requests or decodes move with it.\n got:\n%s\n want:\n%s", got, want)
	}
}
