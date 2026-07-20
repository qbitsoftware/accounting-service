package simplbooks

import (
	"encoding/json"
	"testing"
)

// SimplBooks omits `data` entirely when a list matches nothing — an empty result
// is {"status":200,"duration":0.0022}, not {"data":[]}. Unmarshalling that nil
// RawMessage fails with "unexpected end of JSON input", which broke customer
// lookup the first time an email matched no client.
func TestDecodeDataHandlesAbsentAndNullData(t *testing.T) {
	type row struct {
		Client ClientRecord `json:"Client"`
	}
	cases := map[string]string{
		"data key absent": `{"status":200,"duration":0.0022}`,
		"data null":       `{"status":200,"data":null}`,
		"data empty array": `{"status":200,"data":[]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			resp := mustEnvelope(t, body)
			var rows []row
			if err := resp.decodeData(&rows); err != nil {
				t.Fatalf("decodeData returned error for %s: %v", name, err)
			}
			if len(rows) != 0 {
				t.Errorf("expected 0 rows, got %d", len(rows))
			}
		})
	}

	t.Run("populated data still parses", func(t *testing.T) {
		resp := mustEnvelope(t, `{"status":200,"data":[{"Client":{"id":7,"name":"Klubio"}}]}`)
		var rows []row
		if err := resp.decodeData(&rows); err != nil {
			t.Fatalf("decodeData: %v", err)
		}
		if len(rows) != 1 || rows[0].Client.ID != 7 {
			t.Fatalf("got %+v, want one client with id 7", rows)
		}
	})
}

func mustEnvelope(t *testing.T, body string) *apiResponse {
	t.Helper()
	var resp apiResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("envelope unmarshal: %v", err)
	}
	return &resp
}
