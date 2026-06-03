package merit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// rawPost signs and POSTs to an arbitrary path+query (for endpoints not yet on
// the typed client). Returns status + raw response body.
func (c *Client) rawPost(ctx context.Context, pathAndQuery, body string) (int, string, error) {
	ts := timestamp()
	sig := sign(c.apiID, c.apiKey, ts, body)
	sep := "?"
	if strings.Contains(pathAndQuery, "?") {
		sep = "&"
	}
	reqURL := fmt.Sprintf("%s%s%sApiId=%s&timestamp=%s&signature=%s",
		c.apiURL, pathAndQuery, sep, c.apiID, ts, urlEncodeSignature(sig))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader([]byte(body)))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), nil
}

const (
	phase0CustomerID   = "b6167698-26f1-4aa5-abac-976cd828e297" // "....  OÜ"
	phase0CustomerName = "....  OÜ"
	phase0SwedBankID   = "b7549a02-faac-4d6c-d2c7-08debce783ca" // "Swed" (1010)
)

// TestPhase0Q1_PrepaymentVisibility tests whether a payment with NO invoice
// link creates an unallocated customer advance (ettemaks) and how it reads back
// via getpayments. This is the real Merit prepayment mechanism (no dedicated
// prepayment endpoint exists in the robot API).
func TestPhase0Q1_PrepaymentVisibility(t *testing.T) {
	c := phase0Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// 0) what payment types / accounts exist
	_, pts, _ := c.rawPost(ctx, "v2/getpaymenttypes", "{}")
	t.Logf("PAYMENT TYPES:\n%s", pts)

	// 1) sendpayment with NO InvoiceNo -> does Merit accept it as an advance?
	docNo := "KLUBIO-PP-" + time.Now().UTC().Format("150405")
	body := fmt.Sprintf(`{"BankId":%q,"CustomerName":%q,"InvoiceNo":"","PaymentDate":%q,"Amount":100.00,"CurrencyCode":"EUR","RefNo":%q}`,
		phase0SwedBankID, phase0CustomerName, time.Now().UTC().Format("2006-01-02"), docNo)
	status, resp, err := c.rawPost(ctx, "v2/sendpayment", body)
	if err != nil {
		t.Fatalf("sendpayment transport error: %v", err)
	}
	t.Logf("SENDPAYMENT (no invoice) status=%d docNo=%s\n  request=%s\n  response=%s", status, docNo, body, resp)

	// 2) read payments back regardless, to inspect shape
	time.Sleep(2 * time.Second)
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -7)
	all, err := c.ListPayments(ctx, ListPaymentsParams{
		PeriodStart: start.Format("20060102"), PeriodEnd: end.Format("20060102"),
	})
	if err != nil {
		t.Fatalf("ListPayments: %v", err)
	}
	t.Logf("getpayments (last 7d) returned %d payments", len(all))
	dump(t, "PAYMENTS", all)
}
