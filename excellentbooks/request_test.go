package excellentbooks

import (
	"net/url"
	"strings"
	"testing"
)

// A form without a header Comment is encoded exactly as url.Values.Encode
// encodes it (spaces as %20): every request that never carried one keeps its
// bytes.
func TestEncodeFormWithoutHeaderCommentIsSorted(t *testing.T) {
	fields := map[string]string{
		"set_field.TransDate":         "2026-10-21",
		"set_field.PayMode":           "K",
		"set_field.OKFlag":            "1",
		"set_row_field.0.Comment":     "Makse 2026-0042",
		"set_row_field.0.InvoiceNr":   "260013",
		"set_row_field.0.RecVal":      "15",
		"set_row_field.10.InvoiceNr":  "260014",
		"set_field.CustCode":          "K+1 Õ",
		"set_row_field.0.CustCode":    "KLV1",
		"set_row_field.0.PayDate":     "2026-10-21",
		"set_row_field.0.stp":         "1",
		"set_field.Objects":           "SPORT",
		"set_field.PayCurCode":        "EUR",
		"set_row_field.1.CUPNr":       "707975500",
		"set_row_field.1.RecVal":      "-5",
		"set_row_field.0.Objects":     "TREENER",
		"set_row_field.10.RecVal":     "1",
		"set_row_field.1.CustCode":    "KLV1",
		"set_row_field.1.stp":         "1",
		"set_row_field.10.CustCode":   "KLV1",
		"set_row_field.10.stp":        "1",
		"set_row_field.1.PayDate":     "2026-10-21",
		"set_row_field.10.PayDate":    "2026-10-21",
		"set_row_field.0.BankVal":     "15",
		"set_row_field.0.SuggestVal":  "",
		"set_row_field.0.InvCurncy":   "EUR",
		"set_row_field.0.RecCurncy":   "EUR",
		"set_row_field.0.BankCurncy":  "EUR",
		"set_row_field.0.TaxTemplate": "",
	}
	form := url.Values{}
	for k, v := range fields {
		form.Set(k, v)
	}
	want := strings.ReplaceAll(form.Encode(), "+", "%20")
	if got := encodeForm(fields); got != want {
		t.Fatalf("encodeForm changed a form without a header Comment:\n got: %s\nwant: %s", got, want)
	}
}

// The header Comment follows every other header field — after PayMode, whose
// change makes Excellent Books overwrite the Comment — and precedes the rows.
func TestEncodeFormSendsHeaderCommentAfterPayMode(t *testing.T) {
	got := encodeForm(map[string]string{
		"set_field.Comment":       "KLUBIO-PMT-16",
		"set_field.PayMode":       "K",
		"set_field.OKFlag":        "1",
		"set_field.TransDate":     "2026-10-21",
		"set_row_field.0.Comment": "row text",
		"set_row_field.0.RecVal":  "8",
	})
	want := "set_field.OKFlag=1&set_field.PayMode=K&set_field.TransDate=2026-10-21&set_field.Comment=KLUBIO-PMT-16" +
		"&set_row_field.0.Comment=row%20text&set_row_field.0.RecVal=8"
	if got != want {
		t.Fatalf("encodeForm:\n got: %s\nwant: %s", got, want)
	}
	if only := encodeForm(map[string]string{"set_field.Comment": "a b"}); only != "set_field.Comment=a%20b" {
		t.Fatalf("a lone header Comment = %q", only)
	}
	if rowsOnly := encodeForm(map[string]string{"set_field.Comment": "x", "set_row_field.0.stp": "1"}); rowsOnly != "set_field.Comment=x&set_row_field.0.stp=1" {
		t.Fatalf("header Comment with rows only = %q", rowsOnly)
	}
}
