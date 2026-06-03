package merit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// rawReq lets us vary the HTTP method and inspect response headers, to tell a
// web-server "route not found" 404 from an app-level 405/400.
func (c *Client) rawReq(ctx context.Context, method, pathAndQuery, body string) (int, http.Header, string) {
	ts := timestamp()
	sig := sign(c.apiID, c.apiKey, ts, body)
	sep := "?"
	for i := 0; i < len(pathAndQuery); i++ {
		if pathAndQuery[i] == '?' {
			sep = "&"
			break
		}
	}
	reqURL := fmt.Sprintf("%s%s%sApiId=%s&timestamp=%s&signature=%s",
		c.apiURL, pathAndQuery, sep, c.apiID, ts, urlEncodeSignature(sig))
	req, _ := http.NewRequestWithContext(ctx, method, reqURL, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	s := string(b)
	if len(s) > 120 {
		s = s[:120]
	}
	return resp.StatusCode, resp.Header, s
}

func TestPhase0PrePaymentRoute(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cid := phase0CustomerID
	bid := phase0SwedBankID
	body := `{"Description":"x","DocumentNumber":"KLUBIO-RT","CurrencyCode":"EUR","DocumentDate":"2026-06-03","Amount":100.00}`

	cases := []struct{ method, path string }{
		{"POST", fmt.Sprintf("v2/Banks/%s/PrePayments?customerId=%s", bid, cid)},
		{"POST", fmt.Sprintf("v2/Banks/%s/PrePayments", bid)},
		{"POST", fmt.Sprintf("v2/banks/%s/prepayments?customerId=%s", bid, cid)},
		{"GET", fmt.Sprintf("v2/Banks/%s/PrePayments?customerId=%s", bid, cid)},
		{"POST", "v2/sendsettlement"}, // control: known to exist (app 500)
	}
	for _, tc := range cases {
		st, hdr, short := c.rawReq(ctx, tc.method, tc.path, body)
		server := ""
		ct := ""
		if hdr != nil {
			server = hdr.Get("Server")
			ct = hdr.Get("Content-Type")
		}
		t.Logf("%-4s %-70s -> %d  server=%q ctype=%q body=%q", tc.method, tc.path, st, server, ct, short)
	}
}
