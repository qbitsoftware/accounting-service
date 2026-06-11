package directo

import (
	"encoding/xml"
	"strings"
	"testing"
)

// xmlResultsError is the gate between "Directo answered" and "the document
// is actually in Directo". Only type 0 may pass: the original code special-
// cased types 1/5 and let everything else through, which marked receipts as
// synced when Directo had rejected them with type 12 ("Missing document
// identificator") — phantom payments the accountant could never find.
func TestXMLResultsError(t *testing.T) {
	cases := []struct {
		name       string
		results    XMLResults
		wantErr    bool
		wantSubstr string
	}{
		{
			name:    "type 0 is success",
			results: XMLResults{Results: []XMLResult{{Type: "0"}}},
		},
		{
			name:    "empty type with no error attr is tolerated",
			results: XMLResults{Results: []XMLResult{{}}},
		},
		{
			name:       "type 1 validation failure",
			results:    XMLResults{Results: []XMLResult{{Type: "1", Desc: "korduv arve"}}},
			wantErr:    true,
			wantSubstr: "korduv arve",
		},
		{
			name:       "type 5 unauthorized",
			results:    XMLResults{Results: []XMLResult{{Type: "5", Desc: "no access"}}},
			wantErr:    true,
			wantSubstr: "no access",
		},
		{
			name:       "type 12 missing document identificator is an error",
			results:    XMLResults{Results: []XMLResult{{Type: "12", Desc: "Missing document identificator"}}},
			wantErr:    true,
			wantSubstr: "Missing document identificator",
		},
		{
			name:       "unknown non-zero type is an error",
			results:    XMLResults{Results: []XMLResult{{Type: "7", Msg: "weird state"}}},
			wantErr:    true,
			wantSubstr: "weird state",
		},
		{
			name:       "error attribute fails even with empty type",
			results:    XMLResults{Results: []XMLResult{{Error: "boom"}}},
			wantErr:    true,
			wantSubstr: "boom",
		},
		{
			name: "one bad result among good ones fails the batch",
			results: XMLResults{Results: []XMLResult{
				{Type: "0"},
				{Type: "12", Desc: "Missing document identificator"},
			}},
			wantErr:    true,
			wantSubstr: "Missing document identificator",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := xmlResultsError(c.results)
			if !c.wantErr {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), c.wantSubstr) {
				t.Errorf("error %q should contain %q", err.Error(), c.wantSubstr)
			}
		})
	}
}

// TestReceiptXMLShape pins the marshalled receipt against
// xml_IN_laekumised.xsd. The schema silently ignores unknown attributes
// and elements — the original <line invoiceno= amount=> shape produced a
// hollow receipt with no rows and no Tasumisviis, so the only guard is
// asserting the exact serialised form.
func TestReceiptXMLShape(t *testing.T) {
	wrapper := receiptsXMLWrapper{Receipts: []ReceiptXML{{
		Number:      "7000056",
		Date:        "11.06.2026",
		PaymentMode: "K",
		Confirm:     "1",
		Rows: NewReceiptRows([]ReceiptRowXML{{
			InvoiceNo:    "7000056",
			CustomerCode: "ANNELI_ROOTS",
			Payment:      "80",
			Received:     "80",
			BankCurrency: "EUR",
		}}),
	}}}
	out, err := xml.Marshal(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	want := `<receipts><receipt number="7000056" date="11.06.2026" paymentmode="K" confirm="1"><rows><row invoice="7000056" customer="ANNELI_ROOTS" payment="80" received="80" bankcurrency="EUR"></row></rows></receipt></receipts>`
	if string(out) != want {
		t.Errorf("receipt XML mismatch:\n got: %s\nwant: %s", out, want)
	}
}
