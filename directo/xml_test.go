package directo

import (
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
