package accounting

import (
	"testing"
	"time"

	"github.com/qbitsoftware/accounting-service/simplbooks"
)

// decSB builds a simplbooks.Decimal from a string for test fixtures.
func decSB(s string) simplbooks.Decimal { return simplbooks.Decimal{Decimal: d(s)} }

func TestParseSimplBooksDate(t *testing.T) {
	cases := []struct {
		in   string
		zero bool
	}{
		{"2018-01-01", false},
		{"", true},
		{"0000-00-00", true}, // SimplBooks "unset" sentinel
		{"not-a-date", true},
	}
	for _, c := range cases {
		got := parseSimplBooksDate(c.in)
		if got.IsZero() != c.zero {
			t.Errorf("parseSimplBooksDate(%q) zero=%v, want %v", c.in, got.IsZero(), c.zero)
		}
	}
}

func TestInDateRange(t *testing.T) {
	d := func(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }
	start, end := d("2024-01-01"), d("2024-12-31")
	cases := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"inside", d("2024-06-15"), true},
		{"before", d("2023-12-31"), false},
		{"after", d("2025-01-01"), false},
		{"on start", start, true},
		{"on end", end, true},
		{"zero date never excluded", time.Time{}, true},
	}
	for _, c := range cases {
		if got := inDateRange(c.t, start, end); got != c.want {
			t.Errorf("%s: inDateRange = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSimplBooksRow(t *testing.T) {
	row := simplbooksRow(CreateInvoiceLineInput{
		Code:        "42", // numeric → article_id
		Description: "Monthly fee",
		Quantity:    d("1"),
		UnitPrice:   d("25.50"),
		TaxID:       "5",
		AccountCode: "3",
	})
	if row["article_id"] != 42 {
		t.Errorf("article_id = %v, want 42", row["article_id"])
	}
	if row["vat_type_id"] != 5 {
		t.Errorf("vat_type_id = %v, want 5", row["vat_type_id"])
	}
	if row["income_account_id"] != 3 {
		t.Errorf("income_account_id = %v, want 3", row["income_account_id"])
	}
	if row["name"] != "Monthly fee" {
		t.Errorf("name = %v, want Monthly fee", row["name"])
	}
	if row["price_per_unit"] != "25.5" {
		t.Errorf("price_per_unit = %v, want 25.5", row["price_per_unit"])
	}

	// Non-numeric code must NOT become article_id, and name falls back to code.
	row2 := simplbooksRow(CreateInvoiceLineInput{Code: "MISC", Quantity: d("2"), UnitPrice: d("1")})
	if _, ok := row2["article_id"]; ok {
		t.Error("non-numeric code should not set article_id")
	}
	if row2["name"] != "MISC" {
		t.Errorf("name = %v, want MISC (fallback to code)", row2["name"])
	}
}

func TestMapSimplBooksInvoiceStatus(t *testing.T) {
	cases := []struct {
		name       string
		paid       string
		total      string
		remnant    string
		wantPaid   bool
		wantStatus InvoiceStatus
	}{
		{"paid", "2024-02-01", "100", "0", true, InvoiceStatusPaid},
		{"unpaid", "0000-00-00", "100", "100", false, InvoiceStatusUnpaid},
		{"partial", "0000-00-00", "100", "40", false, InvoiceStatusPartial},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inv := &simplbooks.Invoice{
				ID:       7,
				Number:   "18001",
				Paid:     c.paid,
				TotalSum: decSB(c.total),
				Remnant:  decSB(c.remnant),
			}
			got := mapSimplBooksInvoice(inv)
			if got.Paid != c.wantPaid {
				t.Errorf("Paid = %v, want %v", got.Paid, c.wantPaid)
			}
			if got.Status != c.wantStatus {
				t.Errorf("Status = %v, want %v", got.Status, c.wantStatus)
			}
			if got.ID != "7" {
				t.Errorf("ID = %v, want 7", got.ID)
			}
		})
	}
}

func TestAtoiOrZero(t *testing.T) {
	cases := map[string]int{"42": 42, " 7 ": 7, "": 0, "abc": 0, "0": 0}
	for in, want := range cases {
		if got := atoiOrZero(in); got != want {
			t.Errorf("atoiOrZero(%q) = %d, want %d", in, got, want)
		}
	}
}
