package accounting

import (
	"encoding/json"
	"testing"

	"github.com/qbitsoftware/accounting-service/merit"
)

// A department is opt-in: a caller that sets none must produce exactly the
// payload it produced before the field existed, with no DepartmentCode key on
// the row at all.
func TestMeritRowsCarryDepartmentOnlyWhenSet(t *testing.T) {
	rows, _ := buildRowsAndTaxes([]CreateInvoiceLineInput{
		{Description: "Õppemaks", DepartmentCode: "LE-RY"},
		{Description: "Õppemaks"},
	})
	if rows[0].DepartmentCode != "LE-RY" {
		t.Fatalf("row 0 department = %q, want LE-RY", rows[0].DepartmentCode)
	}
	raw, err := json.Marshal(rows[1])
	if err != nil {
		t.Fatal(err)
	}
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatal(err)
	}
	if _, present := asMap["DepartmentCode"]; present {
		t.Fatalf("row without a department must not send the key: %s", raw)
	}
}

func TestDocumentDepartment(t *testing.T) {
	line := func(code string) CreateInvoiceLineInput { return CreateInvoiceLineInput{DepartmentCode: code} }
	cases := []struct {
		name  string
		lines []CreateInvoiceLineInput
		want  string
	}{
		{"no lines", nil, ""},
		{"none set", []CreateInvoiceLineInput{line(""), line("")}, ""},
		{"all agree", []CreateInvoiceLineInput{line("LE-RY"), line("LE-RY")}, "LE-RY"},
		{"disagree", []CreateInvoiceLineInput{line("LE-RY"), line("HJ")}, ""},
		{"one row without", []CreateInvoiceLineInput{line("LE-RY"), line("")}, ""},
	}
	for _, tc := range cases {
		if got := documentDepartment(tc.lines); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}

	// The header key is omitted too when there is nothing to say.
	raw, _ := json.Marshal(merit.CreateInvoiceRequest{DepartmentCode: documentDepartment(nil)})
	var asMap map[string]any
	_ = json.Unmarshal(raw, &asMap)
	if _, present := asMap["DepartmentCode"]; present {
		t.Fatalf("header without a department must not send the key: %s", raw)
	}
}
