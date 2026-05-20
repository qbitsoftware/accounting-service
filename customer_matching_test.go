package accounting

import "testing"

func TestNormalizeCustomerName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Jaan-Erik", "erik jaan"},
		{"Jaan Erik", "erik jaan"},
		{"OÜ Testfirma", "oü testfirma"},
		{"Testfirma OÜ", "oü testfirma"},
		{"  Multiple   Spaces  ", "multiple spaces"},
		{"A.B.C", "a b c"},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		if got := NormalizeCustomerName(tc.in); got != tc.want {
			t.Errorf("NormalizeCustomerName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMatchCustomerFromList_ByRegNo(t *testing.T) {
	customers := []Customer{
		{ID: "1", Name: "Other Co", RegNo: "11111111", Email: "x@y.com"},
		{ID: "2", Name: "Acme", RegNo: "12345678", Email: "acme@example.com"},
	}
	got, err := MatchCustomerFromList(customers, "Anything", "12345678", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "2" {
		t.Errorf("expected ID=2, got %q", got.ID)
	}
}

func TestMatchCustomerFromList_ByEmailCaseInsensitive(t *testing.T) {
	customers := []Customer{
		{ID: "1", Name: "Acme", Email: "Acme@Example.com"},
	}
	got, err := MatchCustomerFromList(customers, "", "", "acme@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "1" {
		t.Errorf("expected ID=1, got %q", got.ID)
	}
}

func TestMatchCustomerFromList_ByNormalizedName(t *testing.T) {
	customers := []Customer{
		{ID: "1", Name: "OÜ Testfirma"},
	}
	got, err := MatchCustomerFromList(customers, "Testfirma OÜ", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "1" {
		t.Errorf("expected ID=1, got %q", got.ID)
	}
}

func TestMatchCustomerFromList_NoMatch(t *testing.T) {
	customers := []Customer{{ID: "1", Name: "Acme", Email: "acme@example.com"}}
	if _, err := MatchCustomerFromList(customers, "Other", "99999", "other@x.com"); err == nil {
		t.Errorf("expected no-match error, got nil")
	}
}

func TestIsCustomerExistsError(t *testing.T) {
	if !IsCustomerExistsError(stringErr("custexists: bla")) {
		t.Error("expected match on custexists substring")
	}
	if IsCustomerExistsError(stringErr("other error")) {
		t.Error("unexpected match on unrelated error")
	}
	if IsCustomerExistsError(nil) {
		t.Error("nil should not match")
	}
}

func TestIsDuplicateInvoiceError(t *testing.T) {
	if !IsDuplicateInvoiceError(stringErr("Korduv Arve number")) {
		t.Error("expected match on Korduv arve")
	}
	if !IsDuplicateInvoiceError(stringErr("Duplicate INVOICE found")) {
		t.Error("expected case-insensitive match on duplicate invoice")
	}
	if IsDuplicateInvoiceError(stringErr("unrelated")) {
		t.Error("unexpected match on unrelated error")
	}
	if IsDuplicateInvoiceError(nil) {
		t.Error("nil should not match")
	}
}

type stringErr string

func (e stringErr) Error() string { return string(e) }
