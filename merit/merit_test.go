package merit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// --- Auth / Signing Tests ---

func TestSign(t *testing.T) {
	tests := []struct {
		name   string
		apiID  string
		apiKey string
		ts     string
		body   string
		want   string
	}{
		{
			name:   "empty body",
			apiID:  "test-api-id",
			apiKey: "test-api-key",
			ts:     "20240624205902",
			body:   "",
		},
		{
			name:   "with body",
			apiID:  "test-api-id",
			apiKey: "test-api-key",
			ts:     "20240624205902",
			body:   `{"PeriodStart":"20240101","PeriodEnd":"20240331"}`,
		},
		{
			name:   "deterministic - same inputs produce same output",
			apiID:  "670fe52f-558a-4be8-ade0-526e01a106d0",
			apiKey: "secret-key-123",
			ts:     "20240624205902",
			body:   "{}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig1 := sign(tt.apiID, tt.apiKey, tt.ts, tt.body)
			sig2 := sign(tt.apiID, tt.apiKey, tt.ts, tt.body)

			if sig1 == "" {
				t.Error("signature should not be empty")
			}
			if sig1 != sig2 {
				t.Errorf("signatures should be deterministic: got %q and %q", sig1, sig2)
			}
		})
	}
}

func TestSignDifferentInputs(t *testing.T) {
	sig1 := sign("id1", "key1", "20240101000000", "{}")
	sig2 := sign("id2", "key1", "20240101000000", "{}")
	sig3 := sign("id1", "key2", "20240101000000", "{}")
	sig4 := sign("id1", "key1", "20240101000001", "{}")

	sigs := []string{sig1, sig2, sig3, sig4}
	for i := 0; i < len(sigs); i++ {
		for j := i + 1; j < len(sigs); j++ {
			if sigs[i] == sigs[j] {
				t.Errorf("signatures %d and %d should differ, both are %q", i, j, sigs[i])
			}
		}
	}
}

func TestTimestamp(t *testing.T) {
	ts := timestamp()
	if len(ts) != 14 {
		t.Errorf("timestamp should be 14 chars (YYYYMMDDHHmmss), got %d: %q", len(ts), ts)
	}

	_, err := time.Parse("20060102150405", ts)
	if err != nil {
		t.Errorf("timestamp should be parseable: %v", err)
	}
}

func TestURLEncodeSignature(t *testing.T) {
	// Base64 can contain +, /, =
	encoded := urlEncodeSignature("abc+def/ghi=")
	if encoded == "abc+def/ghi=" {
		t.Error("URL encoding should modify special characters")
	}
}

// --- Client Tests ---

func TestNew(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		c := New(Config{
			APIID:  "test-id",
			APIKey: "test-key",
		})

		if c.apiURL != EstoniaURL {
			t.Errorf("expected default URL %q, got %q", EstoniaURL, c.apiURL)
		}
		if c.apiID != "test-id" {
			t.Errorf("expected apiID %q, got %q", "test-id", c.apiID)
		}
		if c.httpClient != http.DefaultClient {
			t.Error("expected default HTTP client")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		customClient := &http.Client{Timeout: 30 * time.Second}
		c := New(Config{
			APIURL:     PolandURL,
			APIID:      "custom-id",
			APIKey:     "custom-key",
			HTTPClient: customClient,
		})

		if c.apiURL != PolandURL {
			t.Errorf("expected URL %q, got %q", PolandURL, c.apiURL)
		}
		if c.httpClient != customClient {
			t.Error("expected custom HTTP client")
		}
	})
}

// --- HTTP Mock Tests ---

func newTestServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := New(Config{
		APIURL: srv.URL + "/api/",
		APIID:  "test-api-id",
		APIKey: "test-api-key",
	})
	return c, srv
}

func TestListInvoices(t *testing.T) {
	invoices := []InvoiceListItem{
		{
			SIHId:        "inv-1",
			InvoiceNo:    "INV-001",
			CustomerName: "Acme Corp",
			TotalAmount:  decimal.NewFromFloat(100.50),
			Paid:         false,
		},
		{
			SIHId:        "inv-2",
			InvoiceNo:    "INV-002",
			CustomerName: "Beta LLC",
			TotalAmount:  decimal.NewFromFloat(200.75),
			Paid:         true,
		},
	}

	client, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v2/getinvoices" {
			t.Errorf("expected path /api/v2/getinvoices, got %s", r.URL.Path)
		}

		// Verify auth params
		q := r.URL.Query()
		if q.Get("ApiId") != "test-api-id" {
			t.Errorf("expected ApiId test-api-id, got %s", q.Get("ApiId"))
		}
		if q.Get("timestamp") == "" {
			t.Error("expected timestamp query param")
		}
		if q.Get("signature") == "" {
			t.Error("expected signature query param")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(invoices)
	})
	defer srv.Close()

	result, err := client.ListInvoices(context.Background(), ListInvoicesParams{
		PeriodStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 invoices, got %d", len(result))
	}
	if result[0].InvoiceNo != "INV-001" {
		t.Errorf("expected INV-001, got %s", result[0].InvoiceNo)
	}
	if !result[0].TotalAmount.Equal(decimal.NewFromFloat(100.50)) {
		t.Errorf("expected 100.50, got %s", result[0].TotalAmount)
	}
}

func TestGetInvoice(t *testing.T) {
	detail := InvoiceDetail{
		SIHId:     "inv-1",
		InvoiceNo: "INV-001",
		Lines: []InvoiceDetailRow{
			{
				SILId:       "line-1",
				ArticleCode: "ITEM-001",
				Quantity:    decimal.NewFromInt(5),
				Price:       decimal.NewFromFloat(20.10),
			},
		},
	}

	client, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/getinvoice" {
			t.Errorf("expected path /api/v2/getinvoice, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(detail)
	})
	defer srv.Close()

	result, err := client.GetInvoice(context.Background(), GetInvoiceParams{ID: "inv-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.InvoiceNo != "INV-001" {
		t.Errorf("expected INV-001, got %s", result.InvoiceNo)
	}
	if len(result.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(result.Lines))
	}
}

func TestCreateInvoice(t *testing.T) {
	client, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/sendinvoice" {
			t.Errorf("expected path /api/v2/sendinvoice, got %s", r.URL.Path)
		}

		var req CreateInvoiceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.InvoiceNo != "INV-100" {
			t.Errorf("expected InvoiceNo INV-100, got %s", req.InvoiceNo)
		}

		resp := CreateInvoiceResponse{
			CustomerID:  "cust-1",
			InvoiceID:   "inv-100",
			InvoiceNo:   "INV-100",
			RefNo:       "123456",
			NewCustomer: false,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	notTD := false
	result, err := client.CreateInvoice(context.Background(), CreateInvoiceRequest{
		Customer: CustomerRef{
			Name:          "Test Customer",
			NotTDCustomer: &notTD,
			CountryCode:   "EE",
		},
		AccountingDoc: DocInvoice,
		InvoiceNo:     "INV-100",
		InvoiceRow: []InvoiceRow{
			{
				Item:     ItemRef{Code: "SVC-1", Description: "Service"},
				Quantity: decimal.NewFromInt(1),
				Price:    decimal.NewFromFloat(50.00),
				TaxID:    "tax-1",
			},
		},
		TaxAmount: []TaxAmountEntry{
			{TaxID: "tax-1", Amount: decimal.NewFromFloat(10.00)},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.InvoiceID != "inv-100" {
		t.Errorf("expected inv-100, got %s", result.InvoiceID)
	}
}

func TestDeleteInvoice(t *testing.T) {
	client, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/deleteinvoice" {
			t.Errorf("expected path /api/v1/deleteinvoice, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	err := client.DeleteInvoice(context.Background(), DeleteInvoiceParams{ID: "inv-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListCustomers(t *testing.T) {
	customers := []CustomerListItem{
		{CustomerID: "cust-1", Name: "Test Corp", RegNo: "12345678"},
	}

	client, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/getcustomers" {
			t.Errorf("expected path /api/v1/getcustomers, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(customers)
	})
	defer srv.Close()

	result, err := client.ListCustomers(context.Background(), ListCustomersParams{Name: "Test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 customer, got %d", len(result))
	}
	if result[0].Name != "Test Corp" {
		t.Errorf("expected Test Corp, got %s", result[0].Name)
	}
}

func TestListTaxes(t *testing.T) {
	taxes := []TaxItem{
		{TaxID: "tax-1", Code: "VAT20", Name: "20% VAT", TaxPct: decimal.NewFromInt(20)}, // TaxID maps to "Id" in JSON
	}

	client, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/gettaxes" {
			t.Errorf("expected path /api/v1/gettaxes, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(taxes)
	})
	defer srv.Close()

	result, err := client.ListTaxes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 tax, got %d", len(result))
	}
	if !result[0].TaxPct.Equal(decimal.NewFromInt(20)) {
		t.Errorf("expected 20%%, got %s", result[0].TaxPct)
	}
}

func TestListPayments(t *testing.T) {
	payments := []PaymentListItem{
		{
			PIHId:           "pay-1",
			DocumentNo:      "PAY-001",
			Amount:          decimal.NewFromFloat(150.00),
			CounterPartType: CounterPartCustomer,
			Direction:       DirectionCustomers,
		},
	}

	client, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/getpayments" {
			t.Errorf("expected path /api/v2/getpayments, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payments)
	})
	defer srv.Close()

	result, err := client.ListPayments(context.Background(), ListPaymentsParams{
		PeriodStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 payment, got %d", len(result))
	}
	if !result[0].Amount.Equal(decimal.NewFromFloat(150.00)) {
		t.Errorf("expected 150.00, got %s", result[0].Amount)
	}
}

func TestAPIError(t *testing.T) {
	client, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Invalid API credentials"))
	})
	defer srv.Close()

	_, err := client.ListTaxes(context.Background())
	if err == nil {
		t.Fatal("expected error for 401 response")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", apiErr.StatusCode)
	}
	if apiErr.Message != "Invalid API credentials" {
		t.Errorf("expected error message, got %q", apiErr.Message)
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	client := New(Config{
		APIURL: "http://127.0.0.1:1/api/",
		APIID:  "test",
		APIKey: "test",
	})

	_, err := client.ListTaxes(ctx)
	if err == nil {
		t.Fatal("expected error due to context cancellation")
	}
}

func TestRequestContentType(t *testing.T) {
	client, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	})
	defer srv.Close()

	client.ListTaxes(context.Background())
}

func TestListPurchases(t *testing.T) {
	purchases := []PurchaseListItem{
		{PIHId: "pi-1", BillNo: "BILL-001", VendorName: "Supplier Inc"},
	}

	client, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/getpurchorders" {
			t.Errorf("expected path /api/v2/getpurchorders, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(purchases)
	})
	defer srv.Close()

	result, err := client.ListPurchases(context.Background(), ListPurchasesParams{
		PeriodStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 purchase, got %d", len(result))
	}
	if result[0].BillNo != "BILL-001" {
		t.Errorf("expected BILL-001, got %s", result[0].BillNo)
	}
}

func TestListVendors(t *testing.T) {
	vendors := []VendorListItem{
		{VendorID: "v-1", Name: "Vendor One", RegNo: "87654321"},
	}

	client, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/getvendors" {
			t.Errorf("expected path /api/v1/getvendors, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(vendors)
	})
	defer srv.Close()

	result, err := client.ListVendors(context.Background(), ListVendorsParams{Name: "Vendor"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].Name != "Vendor One" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestListItems(t *testing.T) {
	items := []ItemListItem{
		{ItemID: "i-1", Code: "ITEM-001", Name: "Widget"},
	}

	client, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/getitems" {
			t.Errorf("expected path /api/v1/getitems, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	})
	defer srv.Close()

	result, err := client.ListItems(context.Background(), ListItemsParams{Code: "ITEM"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].Code != "ITEM-001" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestListAccounts(t *testing.T) {
	accounts := []AccountItem{
		{AccountID: "acc-1", Code: "1000", Name: "Cash"},
	}

	client, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/getaccounts" {
			t.Errorf("expected path /api/v1/getaccounts, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(accounts)
	})
	defer srv.Close()

	result, err := client.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].Code != "1000" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestProfitLoss(t *testing.T) {
	report := FinancialReport{
		Data: []FinancialReportRow{
			{
				RDid:        1,
				Description: "Revenue",
				RowType:     3,
				Balance:     []decimal.Decimal{decimal.NewFromFloat(10000.00)},
			},
		},
	}

	client, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/getprofitrep" {
			t.Errorf("expected path /api/v1/getprofitrep, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(report)
	})
	defer srv.Close()

	result, err := client.ProfitLoss(context.Background(), ProfitLossParams{
		EndDate:  "20251231",
		PerCount: 12,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 1 || result.Data[0].Description != "Revenue" {
		t.Errorf("unexpected result: %+v", result)
	}
}
