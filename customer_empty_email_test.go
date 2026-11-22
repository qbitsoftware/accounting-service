package accounting

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// A document with no recipient e-mail must not be booked to whichever customer
// of the company has no e-mail. Merit's list here holds one such stranger.
func TestFindOrCreateWithoutEmailNeverTakesAStranger(t *testing.T) {
	var created bool
	httpc := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		body := `[]`
		switch {
		case strings.HasSuffix(r.URL.Path, "getcustomers"):
			body = `[{"CustomerId":"stranger","Name":"....  OÜ","Email":"","RegNo":"10000000"}]`
		case strings.HasSuffix(r.URL.Path, "sendcustomer"):
			created = true
			body = `{"Id":"walk-in","Name":"Private customer"}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(bytes.NewReader([]byte(body))), Request: r}, nil
	})}
	c, err := NewClient(Config{Provider: "merit", APIID: "id", APIKey: "key", Region: "ee", HTTPClient: httpc})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Customers.FindOrCreate(context.Background(), "", CreateCustomerInput{Name: "Private customer"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == "stranger" || !created {
		t.Fatalf("customer = %+v (created %v), want a new card, not the customer with no e-mail", got, created)
	}
	if _, err := c.provider.FindCustomerByEmail(context.Background(), ""); !IsNotFound(err) {
		t.Errorf("FindCustomerByEmail(\"\") = %v, want not found", err)
	}
}
