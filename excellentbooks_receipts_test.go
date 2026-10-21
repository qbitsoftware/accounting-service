package accounting

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/qbitsoftware/accounting-service/excellentbooks"
)

// --- fixtures ---

// ebRec renders one IPVc record; rows are ebRow outputs.
func ebRec(serNr, date, seq, okFlag string, rows ...string) string {
	return `{"@sequence":"` + seq + `","@url":"/api/1/IPVc/` + serNr + `","SerNr":"` + serNr + `","TransDate":"` + date +
		`","PayMode":"P2","OKFlag":"` + okFlag + `","PayCurCode":"EUR","rows":[` + strings.Join(rows, ",") + `]}`
}

func ebInvRow(inv, cust, val string) string {
	return `{"stp":"1","InvoiceNr":"` + inv + `","CustCode":"` + cust + `","RecVal":"` + val + `"}`
}

// ebPage wraps records in IPVc's data envelope.
func ebPage(seq string, recs ...string) string {
	s := `{"data":{"@register":"IPVc"`
	if seq != "" {
		s += `,"@sequence":"` + seq + `"`
	}
	return s + `,"IPVc":[` + strings.Join(recs, ",") + `]}}`
}

// ebOffsetServer answers a date-window read from recs like EB is documented
// to: limit/offset over one sorted list. before, when set, runs first and may
// change recs between requests.
func ebOffsetServer(recs *[]string, seq func(n int) string, before func(n int, q url.Values)) func(int, ebWire) (int, string) {
	return func(n int, w ebWire) (int, string) {
		q, _ := url.ParseQuery(w.Query)
		if before != nil {
			before(n, q)
		}
		limit, _ := strconv.Atoi(q.Get("limit"))
		offset, _ := strconv.Atoi(q.Get("offset"))
		all := *recs
		if offset > len(all) {
			offset = len(all)
		}
		end := offset + limit
		if end > len(all) {
			end = len(all)
		}
		return 200, ebPage(seq(n), all[offset:end]...)
	}
}

func numberedRecs(n int) []string {
	out := make([]string, n)
	for i := range out {
		ser := strconv.Itoa(2001 + i)
		out[i] = ebRec(ser, fmt.Sprintf("2026-09-%02d", 1+i%28), strconv.Itoa(7001+i), "1", ebInvRow("26"+ser, "K1", "10"))
	}
	return out
}

func receiptIDs(rs []Receipt) string {
	ids := make([]string, len(rs))
	for i, r := range rs {
		ids[i] = r.ID
	}
	return strings.Join(ids, ",")
}

// --- CreateReceipt ---

const ebCreatePaymentGoldenWire = `POST IPVc set_field.OKFlag=1&set_field.PayCurCode=EUR&set_field.PayMode=P2&set_field.TransDate=2026-09-03&set_row_field.0.CustCode=K7224&set_row_field.0.InvoiceNr=260123&set_row_field.0.PayDate=2026-09-03&set_row_field.0.RecVal=25.5&set_row_field.0.stp=1`

func ebReceiptInput() CreatePaymentInput {
	return CreatePaymentInput{
		CustomerName: "Mari Maasikas",
		CustomerCode: "K7224",
		InvoiceNo:    "260123",
		PaymentDate:  ebDay(2026, 9, 3),
		Amount:       d("25.50"),
		Currency:     "EUR",
		BankID:       "P2",
	}
}

func TestEBCreateReceiptWire(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) { return 200, ebCreated("300001") })
	in := ebReceiptInput()
	in.Document = "KLUBIO-PMT-2002386"
	in.Comment = "Makse 2026-0042"
	got, err := p.CreateReceipt(ebCtx, in)
	if err != nil {
		t.Fatalf("CreateReceipt: %v", err)
	}
	assertEBWire(t, f,
		`POST IPVc set_field.OKFlag=1&set_field.PayCurCode=EUR&set_field.PayMode=P2&set_field.TransDate=2026-09-03&set_field.Comment=KLUBIO-PMT-2002386&set_row_field.0.Comment=Makse%202026-0042&set_row_field.0.CustCode=K7224&set_row_field.0.InvoiceNr=260123&set_row_field.0.PayDate=2026-09-03&set_row_field.0.RecVal=25.5&set_row_field.0.stp=1`,
	)
	if got.ID != "300001" || got.Number != "300001" || !got.Amount.Equal(d("25.5")) || got.InvoiceID != "260123" || got.CustomerID != "K7224" {
		t.Fatalf("CreatedReceipt = %+v", got)
	}
}

// Without Document and Comment, CreateReceipt sends CreatePayment's request
// byte for byte.
func TestEBCreateReceiptWithoutTagsIsCreatePayment(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) { return 200, ebCreated("300001") })
	if _, err := p.CreateReceipt(ebCtx, ebReceiptInput()); err != nil {
		t.Fatalf("CreateReceipt: %v", err)
	}
	assertEBWire(t, f, ebCreatePaymentGoldenWire)
}

func TestEBCreateReceiptResolvedInvoice(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) { return 200, ebCreated("300009") })
	in := ebReceiptInput()
	in.InvoiceNo, in.CustomerCode = "", ""
	in.ResolvedInvoice = &ResolvedProviderInvoice{ID: "260123", CustomerID: "K7224", Currency: "EUR"}
	in.ExpectInvoiceID = "260123"
	got, err := p.CreateReceipt(ebCtx, in)
	if err != nil {
		t.Fatalf("CreateReceipt: %v", err)
	}
	assertEBWire(t, f, ebCreatePaymentGoldenWire)
	if got.ID != "300009" || got.InvoiceID != "260123" || got.CustomerID != "K7224" {
		t.Fatalf("CreatedReceipt = %+v", got)
	}
}

func TestEBCreateReceiptRefusesBeforeWriting(t *testing.T) {
	cases := []struct {
		name string
		mod  func(*CreatePaymentInput)
		is   error
	}{
		{"no PayMode", func(in *CreatePaymentInput) { in.BankID = "" }, ErrInvalidInput},
		{"no invoice", func(in *CreatePaymentInput) { in.InvoiceNo = "" }, ErrInvalidInput},
		{"expected another invoice", func(in *CreatePaymentInput) { in.ExpectInvoiceID = "260999" }, ErrProviderInvoiceMismatch},
		{"resolved another invoice", func(in *CreatePaymentInput) {
			in.ResolvedInvoice = &ResolvedProviderInvoice{ID: "260999", CustomerID: "K7224"}
		}, ErrProviderInvoiceMismatch},
		{"resolved without id", func(in *CreatePaymentInput) { in.ResolvedInvoice = &ResolvedProviderInvoice{} }, ErrInvalidInput},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, f := newEBFake(t, nil)
			in := ebReceiptInput()
			c.mod(&in)
			if _, err := p.CreateReceipt(ebCtx, in); !errors.Is(err, c.is) {
				t.Fatalf("err = %v, want %v", err, c.is)
			}
			assertEBWire(t, f)
		})
	}
}

// The receipt exists once EB has accepted the write: an answer that names no
// SerNr — or does not decode at all — is success with an empty id, never an
// error that would invite a second receipt.
func TestEBCreateReceiptAnswerWithoutSerNr(t *testing.T) {
	for name, body := range map[string]string{
		"no IPVc":    `{"data":{"@register":"IPVc","@sequence":"9001"}}`,
		"empty list": `{"data":{"@register":"IPVc","IPVc":[]}}`,
		"no data":    `{"messages":["Saved"]}`,
		"malformed":  `{"data":{"IPVc":[{"SerNr":`,
	} {
		t.Run(name, func(t *testing.T) {
			p, _ := newEBFake(t, func(int, ebWire) (int, string) { return 200, body })
			got, err := p.CreateReceipt(ebCtx, ebReceiptInput())
			if err != nil {
				t.Fatalf("CreateReceipt: %v", err)
			}
			if got.ID != "" || !got.Amount.Equal(d("25.5")) || got.InvoiceID != "260123" {
				t.Fatalf("CreatedReceipt = %+v", got)
			}
		})
	}
}

func TestEBCreateReceiptRejected(t *testing.T) {
	for name, resp := range map[string]struct {
		status int
		body   string
	}{
		"error payload on 200": {200, `{"error":{"@code":"20060","@description":"Summa on suurem kui arve summa 10"}}`},
		"400":                  {400, `{"error":{"@code":"1289","@field":"CUPNr","@description":"puudub"}}`},
		"500":                  {500, `boom`},
	} {
		t.Run(name, func(t *testing.T) {
			p, _ := newEBFake(t, func(int, ebWire) (int, string) { return resp.status, resp.body })
			if got, err := p.CreateReceipt(ebCtx, ebReceiptInput()); err == nil {
				t.Fatalf("CreateReceipt = %+v, want an error", got)
			}
		})
	}
}

// A zero amount is left to EB (CreatePayment's rule); the saved record says
// what it booked.
func TestEBCreateReceiptZeroAmountReadsTheAnswer(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) {
		return 200, ebPage("9002", ebRec("300010", "2026-09-03", "9002", "1", ebInvRow("260123", "K7224", "42.00")))
	})
	in := ebReceiptInput()
	in.Amount = d("0")
	got, err := p.CreateReceipt(ebCtx, in)
	if err != nil {
		t.Fatalf("CreateReceipt: %v", err)
	}
	if strings.Contains(f.wire()[0].Body, "RecVal") {
		t.Fatalf("a zero amount must leave RecVal unsent: %s", f.wire()[0].Body)
	}
	if got.ID != "300010" || !got.Amount.Equal(d("42")) {
		t.Fatalf("CreatedReceipt = %+v", got)
	}
}

// --- ApplyPrepaymentReceipt ---

func TestEBApplyPrepaymentReceiptWire(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) { return 200, ebCreated("300004") })
	in := ApplyPrepaymentInput{
		CustomerCode: "K7224",
		InvoiceNo:    "260123",
		PrepaymentNo: "KLB-PP-3",
		Amount:       d("20"),
		Currency:     "EUR",
		PaymentDate:  ebDay(2026, 9, 5),
		BankID:       "P2",
		Document:     "KLUBIO-APPLY-77",
	}
	got, err := p.ApplyPrepaymentReceipt(ebCtx, in)
	if err != nil {
		t.Fatalf("ApplyPrepaymentReceipt: %v", err)
	}
	in.Document = ""
	if _, err := p.ApplyPrepaymentReceipt(ebCtx, in); err != nil {
		t.Fatalf("ApplyPrepaymentReceipt without Document: %v", err)
	}
	assertEBWire(t, f,
		`POST IPVc set_field.OKFlag=1&set_field.PayCurCode=EUR&set_field.PayMode=P2&set_field.TransDate=2026-09-05&set_field.Comment=KLUBIO-APPLY-77&set_row_field.0.CustCode=K7224&set_row_field.0.InvoiceNr=260123&set_row_field.0.PayDate=2026-09-05&set_row_field.0.RecVal=20&set_row_field.0.stp=1&set_row_field.1.CUPNr=KLB-PP-3&set_row_field.1.CustCode=K7224&set_row_field.1.PayDate=2026-09-05&set_row_field.1.RecVal=-20&set_row_field.1.stp=1`,
		// ApplyPrepayment's golden request, byte for byte.
		`POST IPVc set_field.OKFlag=1&set_field.PayCurCode=EUR&set_field.PayMode=P2&set_field.TransDate=2026-09-05&set_row_field.0.CustCode=K7224&set_row_field.0.InvoiceNr=260123&set_row_field.0.PayDate=2026-09-05&set_row_field.0.RecVal=20&set_row_field.0.stp=1&set_row_field.1.CUPNr=KLB-PP-3&set_row_field.1.CustCode=K7224&set_row_field.1.PayDate=2026-09-05&set_row_field.1.RecVal=-20&set_row_field.1.stp=1`,
	)
	if got.ID != "300004" || got.Number != "300004" || !got.Amount.IsZero() || got.InvoiceID != "260123" || got.CustomerID != "K7224" {
		t.Fatalf("CreatedReceipt = %+v", got)
	}
}

func TestEBApplyPrepaymentReceiptRefusesBeforeWriting(t *testing.T) {
	p, f := newEBFake(t, nil)
	if _, err := p.ApplyPrepaymentReceipt(ebCtx, ApplyPrepaymentInput{CustomerCode: "K1", InvoiceNo: "1", Amount: d("1"), BankID: "P2"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("without a PrepaymentNo: %v", err)
	}
	if _, err := p.ApplyPrepaymentReceipt(ebCtx, ApplyPrepaymentInput{CustomerCode: "K1", InvoiceNo: "1", PrepaymentNo: "A", Amount: d("1")}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("without a PayMode: %v", err)
	}
	assertEBWire(t, f)
}

// --- GetReceipt ---

func TestEBGetReceipt(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) {
		return 200, ebPage("5120", ebRec("1001", "2026-09-01", "5101", "1", ebInvRow("260100", "K1", "45.00")))
	})
	r, err := p.GetReceipt(ebCtx, "1001")
	if err != nil {
		t.Fatalf("GetReceipt: %v", err)
	}
	assertEBWire(t, f, `GET IPVc/1001`)
	if r.ID != "1001" || r.Sequence != "5101" || !r.Amount.Equal(d("45")) || r.CustomerID != "K1" || len(r.Rows) != 1 {
		t.Fatalf("receipt = %+v", r)
	}
}

func TestEBGetReceiptNotFound(t *testing.T) {
	for name, resp := range map[string]struct {
		status int
		body   string
	}{
		"404":        {404, `{"error":{"@code":"404","@description":"Record not found"}}`},
		"404 bare":   {404, `Not Found`},
		"empty list": {200, `{"data":{"@register":"IPVc","IPVc":[]}}`},
		"no IPVc":    {200, `{"data":{"@register":"IPVc","@sequence":"5"}}`},
		"no data":    {200, `{}`},
		"null data":  {200, `{"data":null}`},
	} {
		t.Run(name, func(t *testing.T) {
			p, _ := newEBFake(t, func(int, ebWire) (int, string) { return resp.status, resp.body })
			if _, err := p.GetReceipt(ebCtx, "1001"); !IsNotFound(err) {
				t.Fatalf("GetReceipt = %v, want IsNotFound", err)
			}
		})
	}
}

// Only a 404 or an empty answer means gone. Anything else — an error payload,
// a server error, an answer about another receipt — is an error that is NOT
// not-found, or a caller would conclude a receipt it cannot read is deleted.
func TestEBGetReceiptOtherFailuresAreNotNotFound(t *testing.T) {
	for name, resp := range map[string]struct {
		status int
		body   string
		is     error
	}{
		"error payload": {200, `{"error":{"@code":"1","@description":"Register locked"}}`, nil},
		"500":           {500, `boom`, nil},
		"other receipt": {200, ebPage("1", ebRec("1002", "2026-09-02", "5102", "1", ebInvRow("1", "K1", "1"))), ErrIDFilterIgnored},
	} {
		t.Run(name, func(t *testing.T) {
			p, _ := newEBFake(t, func(int, ebWire) (int, string) { return resp.status, resp.body })
			_, err := p.GetReceipt(ebCtx, "1001")
			if err == nil || IsNotFound(err) {
				t.Fatalf("GetReceipt = %v, want an error that is not IsNotFound", err)
			}
			if resp.is != nil && !errors.Is(err, resp.is) {
				t.Fatalf("GetReceipt = %v, want %v", err, resp.is)
			}
		})
	}
}

// --- ListInvoiceReceipts ---

func TestEBListInvoiceReceiptsFiltersRowsInGo(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) { return 200, ebReceiptFixture })
	all, err := p.ListInvoiceReceipts(ebCtx, ListInvoiceReceiptsInput{CustomerID: "K1", Since: ebDay(2026, 8, 1), Until: ebDay(2026, 9, 30)})
	if err != nil {
		t.Fatalf("ListInvoiceReceipts: %v", err)
	}
	if got := receiptIDs(all); got != "1001,1002" {
		t.Fatalf("K1 receipts = %s, want 1001,1002", got)
	}
	one, err := p.ListInvoiceReceipts(ebCtx, ListInvoiceReceiptsInput{CustomerID: "K1", InvoiceID: "260101"})
	if err != nil {
		t.Fatalf("ListInvoiceReceipts by invoice: %v", err)
	}
	if got := receiptIDs(one); got != "1002" || len(one[0].Rows) != 2 {
		t.Fatalf("K1/260101 = %s with %d rows, want 1002 whole (2 rows)", got, len(one[0].Rows))
	}
	split, err := p.ListInvoiceReceipts(ebCtx, ListInvoiceReceiptsInput{CustomerID: "K4", InvoiceID: "260104"})
	if err != nil {
		t.Fatalf("ListInvoiceReceipts split: %v", err)
	}
	if got := receiptIDs(split); got != "1005" || len(split[0].Rows) != 2 || !split[0].AmountOn("260104").Equal(d("50")) {
		t.Fatalf("K4/260104 = %+v, want 1005 whole", split)
	}
	none, err := p.ListInvoiceReceipts(ebCtx, ListInvoiceReceiptsInput{CustomerID: "K1", InvoiceID: "260104"})
	if err != nil || len(none) != 0 {
		t.Fatalf("another customer's invoice = %v, %v; want none", none, err)
	}
	assertEBWire(t, f,
		`GET IPVc?limit=500&range=2026-08-01%3A2026-09-30&sort=TransDate`,
		`GET IPVc?limit=500&range=2000-01-01%3A&sort=TransDate`,
		`GET IPVc?limit=500&range=2000-01-01%3A&sort=TransDate`,
		`GET IPVc?limit=500&range=2000-01-01%3A&sort=TransDate`,
	)
}

func TestEBListInvoiceReceiptsRequiresCustomer(t *testing.T) {
	p, f := newEBFake(t, nil)
	if _, err := p.ListInvoiceReceipts(ebCtx, ListInvoiceReceiptsInput{InvoiceID: "1"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	assertEBWire(t, f)
}

// Pages overlap by one record: each later page starts on the record that ended
// the previous one, which is how a page that moved is caught.
func TestEBListInvoiceReceiptsPaginates(t *testing.T) {
	recs := numberedRecs(8)
	p, f := newEBFake(t, ebOffsetServer(&recs, func(int) string { return "7100" }, nil))
	p.pageSize = 3
	got, err := p.ListInvoiceReceipts(ebCtx, ListInvoiceReceiptsInput{CustomerID: "K1", Since: ebDay(2026, 9, 1), Until: ebDay(2026, 9, 30)})
	if err != nil {
		t.Fatalf("ListInvoiceReceipts: %v", err)
	}
	if ids := receiptIDs(got); ids != "2001,2002,2003,2004,2005,2006,2007,2008" {
		t.Fatalf("ids = %s, want all eight once", ids)
	}
	assertEBWire(t, f,
		`GET IPVc?limit=3&range=2026-09-01%3A2026-09-30&sort=TransDate`,
		`GET IPVc?limit=3&offset=2&range=2026-09-01%3A2026-09-30&sort=TransDate`,
		`GET IPVc?limit=3&offset=4&range=2026-09-01%3A2026-09-30&sort=TransDate`,
		`GET IPVc?limit=3&offset=6&range=2026-09-01%3A2026-09-30&sort=TransDate`,
	)
}

func TestEBListInvoiceReceiptsCap(t *testing.T) {
	recs := numberedRecs(20)
	p, f := newEBFake(t, ebOffsetServer(&recs, func(int) string { return "1" }, nil))
	p.pageSize = 2
	got, err := p.ListInvoiceReceipts(ebCtx, ListInvoiceReceiptsInput{CustomerID: "K1"})
	if !errors.Is(err, ErrTooManyPages) || got != nil {
		t.Fatalf("ListInvoiceReceipts = %v, %v; want ErrTooManyPages and nothing", got, err)
	}
	if n := len(f.wire()); n != ebDefaultMaxPages {
		t.Fatalf("%d requests, want the default cap of %d", n, ebDefaultMaxPages)
	}

	recs3 := numberedRecs(9)
	p, f = newEBFake(t, ebOffsetServer(&recs3, func(int) string { return "1" }, nil))
	p.pageSize = 3
	if _, err := p.ListInvoiceReceipts(ebCtx, ListInvoiceReceiptsInput{CustomerID: "K1", MaxPages: 2}); !errors.Is(err, ErrTooManyPages) {
		t.Fatalf("MaxPages 2: %v, want ErrTooManyPages", err)
	}
	if n := len(f.wire()); n != 2 {
		t.Fatalf("%d requests, want 2", n)
	}
}

// A receipt saved earlier in the order between two pages shifts the rest: the
// read starts over once and succeeds when the list holds still.
func TestEBListInvoiceReceiptsShiftRetriesOnce(t *testing.T) {
	recs := numberedRecs(5)
	inserted := ebRec("1999", "2026-08-31", "7999", "1", ebInvRow("261999", "K1", "10"))
	p, f := newEBFake(t, ebOffsetServer(&recs, func(int) string { return "1" }, func(n int, _ url.Values) {
		if n == 1 {
			recs = append([]string{inserted}, recs...)
		}
	}))
	p.pageSize = 3
	got, err := p.ListInvoiceReceipts(ebCtx, ListInvoiceReceiptsInput{CustomerID: "K1"})
	if err != nil {
		t.Fatalf("ListInvoiceReceipts: %v", err)
	}
	if ids := receiptIDs(got); ids != "1999,2001,2002,2003,2004,2005" {
		t.Fatalf("ids = %s", ids)
	}
	if n := len(f.wire()); n != 2+3 {
		t.Fatalf("%d requests, want 2 (shifted) + 3 (clean)", n)
	}
}

func TestEBListInvoiceReceiptsShiftFails(t *testing.T) {
	recs := numberedRecs(5)
	k := 0
	p, _ := newEBFake(t, ebOffsetServer(&recs, func(int) string { return "1" }, func(n int, q url.Values) {
		if q.Get("offset") != "" { // every second page finds a new receipt in front of it
			k++
			recs = append([]string{ebRec(strconv.Itoa(1900+k), "2026-08-01", "1", "1", ebInvRow("1", "K1", "1"))}, recs...)
		}
	}))
	p.pageSize = 3
	if got, err := p.ListInvoiceReceipts(ebCtx, ListInvoiceReceiptsInput{CustomerID: "K1"}); !errors.Is(err, ErrPageShifted) || got != nil {
		t.Fatalf("ListInvoiceReceipts = %v, %v; want ErrPageShifted and nothing", got, err)
	}
}

// A server that ignores offset answers page one again: the pages cannot be
// joined and nothing is returned.
func TestEBListInvoiceReceiptsOffsetIgnored(t *testing.T) {
	recs := numberedRecs(5)
	p, _ := newEBFake(t, func(n int, w ebWire) (int, string) { return 200, ebPage("1", recs[:3]...) })
	p.pageSize = 3
	if _, err := p.ListInvoiceReceipts(ebCtx, ListInvoiceReceiptsInput{CustomerID: "K1"}); !errors.Is(err, ErrPageShifted) {
		t.Fatalf("err = %v, want ErrPageShifted", err)
	}
}

// --- ListPaymentChanges: the date window ---

func TestEBListPaymentChangesWindow(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) { return 200, ebReceiptFixture })
	got, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{Since: ebDay(2026, 9, 1), Until: ebDay(2026, 9, 30), Precise: true, ClientsOnly: true, WithDeletions: true})
	if err != nil {
		t.Fatalf("ListPaymentChanges: %v", err)
	}
	assertEBWire(t, f, `GET IPVc?limit=500&range=2026-09-01%3A2026-09-30&sort=TransDate`)
	if r := renderSAPayments(got.Payments); r != ebPaymentsFixtureWant {
		t.Fatalf("Payments must be ListPayments' projection\n got:\n%s\n want:\n%s", r, ebPaymentsFixtureWant)
	}
	if len(got.Receipts) != 8 || got.Sequence != "5120" || got.Deleted != nil || got.DeletedComplete {
		t.Fatalf("changes = %d receipts, seq %q, deleted %v/%v", len(got.Receipts), got.Sequence, got.Deleted, got.DeletedComplete)
	}
}

// The window's cursor is the FIRST page's sequence: a change made while the
// read runs is read again next time rather than skipped.
func TestEBListPaymentChangesWindowSequenceIsTheFirstPages(t *testing.T) {
	recs := numberedRecs(5)
	p, _ := newEBFake(t, ebOffsetServer(&recs, func(n int) string { return strconv.Itoa(8000 + n) }, nil))
	p.pageSize = 3
	got, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{Since: ebDay(2026, 9, 1)})
	if err != nil {
		t.Fatalf("ListPaymentChanges: %v", err)
	}
	if got.Sequence != "8000" || len(got.Receipts) != 5 || len(got.Payments) != 5 {
		t.Fatalf("seq %q, %d receipts, %d payments", got.Sequence, len(got.Receipts), len(got.Payments))
	}
}

func TestEBListPaymentChangesNeedsAStart(t *testing.T) {
	p, f := newEBFake(t, nil)
	if _, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	assertEBWire(t, f)
}

// --- ListPaymentChanges: the sequence feed ---

func TestEBListPaymentChangesFeedWire(t *testing.T) {
	deleted := `{"data":{"@register":"IPVc","@sequence":"5121","IPVc":[{"SerNr":"1009","@sequence":"5115"}]}}`
	p, f := newEBFake(t, func(n int, w ebWire) (int, string) {
		if strings.Contains(w.Query, "deletes_after") {
			return 200, deleted
		}
		return 200, ebPage("5120",
			ebRec("1001", "2026-09-01", "5101", "1", ebInvRow("260100", "K1", "45.00")),
			ebRec("1002", "2026-01-15", "5110", "1", ebInvRow("250100", "K2", "12.00")))
	})
	got, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{SinceSequence: "5100", WithDeletions: true, Since: ebDay(2026, 9, 1)})
	if err != nil {
		t.Fatalf("ListPaymentChanges: %v", err)
	}
	assertEBWire(t, f,
		`GET IPVc?limit=500&updates_after=5100`,
		`GET IPVc?deletes_after=5100&limit=500`,
	)
	if receiptIDs(got.Receipts) != "1001,1002" || len(got.Payments) != 2 || got.Sequence != "5120" {
		t.Fatalf("changes = %s, %d payments, seq %q", receiptIDs(got.Receipts), len(got.Payments), got.Sequence)
	}
	if strings.Join(got.Deleted, ",") != "1009" || !got.DeletedComplete {
		t.Fatalf("deleted = %v complete=%v, want [1009] complete", got.Deleted, got.DeletedComplete)
	}
}

func TestEBListPaymentChangesFeedWithoutDeletions(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) { return 200, ebPage("5120") })
	got, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{SinceSequence: "5100"})
	if err != nil {
		t.Fatalf("ListPaymentChanges: %v", err)
	}
	assertEBWire(t, f, `GET IPVc?limit=500&updates_after=5100`)
	if len(got.Receipts) != 0 || got.Sequence != "5120" || got.Deleted != nil || got.DeletedComplete {
		t.Fatalf("changes = %+v", got)
	}
}

// A refusal is ErrChangeFeedUnsupported; a failure to answer is not.
func TestEBListPaymentChangesFeedRefusalVsFailure(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		unsupported bool
	}{
		{"error payload on 200", 200, `{"error":{"@code":"1857","@description":"Register has no synchronisation index"}}`, true},
		{"400", 400, `{"error":{"@code":"1","@description":"updates_after not allowed"}}`, true},
		{"404", 404, `Not Found`, true},
		{"500", 500, `boom`, false},
		{"502", 502, `bad gateway`, false},
		{"503", 503, `busy`, false},
		{"500 with an error payload", 500, `{"error":{"@code":"1","@description":"internal"}}`, false},
		{"401", 401, `no`, false},
		{"403", 403, `{"error":{"@code":"1","@description":"forbidden"}}`, false},
		{"408", 408, `timeout`, false},
		{"429", 429, `slow down`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, _ := newEBFake(t, func(int, ebWire) (int, string) { return c.status, c.body })
			_, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{SinceSequence: "5100", WithDeletions: true})
			if err == nil {
				t.Fatal("want an error")
			}
			if got := errors.Is(err, ErrChangeFeedUnsupported); got != c.unsupported {
				t.Fatalf("errors.Is(ErrChangeFeedUnsupported) = %v, want %v (%v)", got, c.unsupported, err)
			}
			if c.unsupported {
				var apiErr *excellentbooks.APIError
				if !errors.As(err, &apiErr) || IsNotFound(err) {
					t.Fatalf("a refusal keeps the EB error and is not not-found: %v", err)
				}
			}
		})
	}
}

func TestEBListPaymentChangesFeedTransportErrorIsNotARefusal(t *testing.T) {
	p, _ := newEBFake(t, nil)
	p.client = excellentbooks.New(excellentbooks.Config{BaseURL: "http://127.0.0.1:1", CompanyCode: "1"})
	_, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{SinceSequence: "5100"})
	if err == nil || errors.Is(err, ErrChangeFeedUnsupported) {
		t.Fatalf("err = %v, want an ordinary error", err)
	}
}

// A register that ignores updates_after answers its records from the start,
// below the cursor: that proves the parameter was not applied.
func TestEBListPaymentChangesFeedIgnored(t *testing.T) {
	p, _ := newEBFake(t, func(int, ebWire) (int, string) {
		return 200, ebPage("5120", ebRec("1", "2020-01-01", "12", "1", ebInvRow("1", "K1", "1")))
	})
	if _, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{SinceSequence: "5100"}); !errors.Is(err, ErrChangeFeedUnsupported) {
		t.Fatalf("err = %v, want ErrChangeFeedUnsupported", err)
	}
}

// The feed pages by the last record's @sequence, not by offset: a receipt
// changed during the read moves to the end of the order and is read, never
// skipped.
func TestEBListPaymentChangesFeedPaginates(t *testing.T) {
	type rec struct{ ser, seq string }
	all := []rec{{"1", "101"}, {"2", "102"}, {"3", "105"}, {"4", "107"}, {"5", "110"}}
	p, f := newEBFake(t, func(n int, w ebWire) (int, string) {
		q, _ := url.ParseQuery(w.Query)
		after, _ := strconv.Atoi(q.Get("updates_after"))
		limit, _ := strconv.Atoi(q.Get("limit"))
		var page []string
		for _, r := range all {
			if s, _ := strconv.Atoi(r.seq); s > after && len(page) < limit {
				page = append(page, ebRec(r.ser, "2026-09-01", r.seq, "1", ebInvRow("26"+r.ser, "K1", "1")))
			}
		}
		return 200, ebPage(strconv.Itoa(200+n), page...)
	})
	p.pageSize = 2
	got, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{SinceSequence: "100"})
	if err != nil {
		t.Fatalf("ListPaymentChanges: %v", err)
	}
	assertEBWire(t, f,
		`GET IPVc?limit=2&updates_after=100`,
		`GET IPVc?limit=2&updates_after=102`,
		`GET IPVc?limit=2&updates_after=107`,
	)
	if ids := receiptIDs(got.Receipts); ids != "1,2,3,4,5" || got.Sequence != "202" {
		t.Fatalf("ids %s seq %q, want 1..5 and the last page's data@sequence 202", ids, got.Sequence)
	}
}

func TestEBListPaymentChangesFeedNotAdvancing(t *testing.T) {
	p, _ := newEBFake(t, func(int, ebWire) (int, string) {
		return 200, ebPage("9", ebRec("1", "2026-09-01", "100", "1", ebInvRow("1", "K1", "1")), ebRec("2", "2026-09-01", "100", "1", ebInvRow("2", "K1", "1")))
	})
	p.pageSize = 2
	if _, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{SinceSequence: "100"}); !errors.Is(err, ErrChangeFeedUnsupported) {
		t.Fatalf("err = %v, want ErrChangeFeedUnsupported", err)
	}
}

// The feed pages by the last record's @sequence, which skips nothing only
// while every page is in ascending order. A register answering in any other
// order moves the cursor past receipts it never returned — newest first, the
// first page ends at 104 and 101..103 are never read; by SerNr it ends at 105
// and 102..104 are never read — so a page out of order is refused, not read.
func TestEBListPaymentChangesFeedOutOfOrder(t *testing.T) {
	type rec struct {
		ser string
		seq int
	}
	all := []rec{{"1", 101}, {"2", 106}, {"3", 105}, {"4", 102}, {"5", 103}, {"6", 104}}
	cases := []struct {
		name string
		less func(a, b rec) bool
	}{
		{"newest first", func(a, b rec) bool { return a.seq > b.seq }},
		{"by SerNr", func(a, b rec) bool { return a.ser < b.ser }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, _ := newEBFake(t, func(n int, w ebWire) (int, string) {
				q, _ := url.ParseQuery(w.Query)
				after, _ := strconv.Atoi(q.Get("updates_after"))
				limit, _ := strconv.Atoi(q.Get("limit"))
				var due []rec
				for _, r := range all {
					if r.seq > after {
						due = append(due, r)
					}
				}
				sort.SliceStable(due, func(i, j int) bool { return c.less(due[i], due[j]) })
				var page []string
				for _, r := range due {
					if len(page) == limit {
						break
					}
					page = append(page, ebRec(r.ser, "2026-09-01", strconv.Itoa(r.seq), "1", ebInvRow("26"+r.ser, "K1", "1")))
				}
				return 200, ebPage(strconv.Itoa(200+n), page...)
			})
			p.pageSize = 3
			got, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{SinceSequence: "100"})
			if !errors.Is(err, ErrChangeFeedUnsupported) || got != nil {
				ids := ""
				if got != nil {
					ids = receiptIDs(got.Receipts)
				}
				t.Fatalf("ListPaymentChanges = [%s], %v; want ErrChangeFeedUnsupported and nothing", ids, err)
			}
		})
	}
}

// A record whose @sequence is not a number cannot be placed in the order the
// cursor relies on, wherever it sits on the page.
func TestEBListPaymentChangesFeedNonNumericSequence(t *testing.T) {
	for _, seq := range []string{"", "abc", "1e3", "-5"} {
		t.Run(fmt.Sprintf("%q", seq), func(t *testing.T) {
			p, _ := newEBFake(t, func(int, ebWire) (int, string) {
				return 200, ebPage("130",
					ebRec("1", "2026-09-01", "110", "1", ebInvRow("1", "K1", "1")),
					ebRec("2", "2026-09-01", seq, "1", ebInvRow("2", "K1", "1")),
					ebRec("3", "2026-09-01", "120", "1", ebInvRow("3", "K1", "1")))
			})
			if got, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{SinceSequence: "100"}); !errors.Is(err, ErrChangeFeedUnsupported) || got != nil {
				ids := ""
				if got != nil {
					ids = receiptIDs(got.Receipts)
				}
				t.Fatalf("ListPaymentChanges = [%s], %v; want ErrChangeFeedUnsupported and nothing", ids, err)
			}
		})
	}
}

// An error payload on a 2xx is a refusal of updates_after only on the request
// that carried it; the same answer to a date-window read is an ordinary error.
func TestEBErrorPayloadIsARefusalOnlyOnTheFeed(t *testing.T) {
	const refused = `{"error":{"@code":"1857","@description":"Register has no synchronisation index"}}`
	answer := func(int, ebWire) (int, string) { return 200, refused }

	p, f := newEBFake(t, answer)
	_, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{Since: ebDay(2026, 9, 1)})
	if err == nil || errors.Is(err, ErrChangeFeedUnsupported) {
		t.Fatalf("window: err = %v, want an ordinary error", err)
	}
	assertEBWire(t, f, `GET IPVc?limit=500&range=2026-09-01%3A&sort=TransDate`)

	p, f = newEBFake(t, answer)
	_, err = p.ListInvoiceReceipts(ebCtx, ListInvoiceReceiptsInput{CustomerID: "K1", Since: ebDay(2026, 9, 1)})
	if err == nil || errors.Is(err, ErrChangeFeedUnsupported) {
		t.Fatalf("invoice receipts: err = %v, want an ordinary error", err)
	}
	for _, w := range f.wire() {
		if strings.Contains(w.Query, "updates_after") {
			t.Fatalf("invoice receipts sent updates_after: %s", w)
		}
	}

	p, f = newEBFake(t, answer)
	_, err = p.ListPaymentChanges(ebCtx, PaymentChangesInput{SinceSequence: "5100"})
	if !errors.Is(err, ErrChangeFeedUnsupported) {
		t.Fatalf("feed: err = %v, want ErrChangeFeedUnsupported", err)
	}
	assertEBWire(t, f, `GET IPVc?limit=500&updates_after=5100`)
}

func TestEBListPaymentChangesFeedSequenceFallbacks(t *testing.T) {
	p, _ := newEBFake(t, func(int, ebWire) (int, string) {
		return 200, ebPage("", ebRec("1", "2026-09-01", "130", "1", ebInvRow("1", "K1", "1")))
	})
	got, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{SinceSequence: "100"})
	if err != nil || got.Sequence != "130" {
		t.Fatalf("no data@sequence: seq %q (%v), want the last record's 130", got.Sequence, err)
	}
	p, _ = newEBFake(t, func(int, ebWire) (int, string) { return 200, ebPage("") })
	got, err = p.ListPaymentChanges(ebCtx, PaymentChangesInput{SinceSequence: "100"})
	if err != nil || got.Sequence != "100" {
		t.Fatalf("nothing at all: seq %q (%v), want the cursor itself", got.Sequence, err)
	}
}

// deletes_after's answer is believed only when it proves itself a deletion
// list. Every other answer leaves Deleted empty and DeletedComplete false,
// and is not an error.
func TestEBListPaymentChangesDeletesAfterShapes(t *testing.T) {
	live := ebPage("5120", ebRec("1001", "2026-09-01", "5101", "1", ebInvRow("260100", "K1", "45.00")))
	cases := []struct {
		name     string
		status   int
		body     string
		want     string
		complete bool
	}{
		{"one deleted", 200, `{"data":{"@register":"IPVc","@sequence":"5121","IPVc":[{"SerNr":"1009","@sequence":"5115"}]}}`, "1009", true},
		{"numeric fields", 200, `{"data":{"IPVc":[{"SerNr":1009,"@sequence":5115}]}}`, "1009", true},
		{"single object", 200, `{"data":{"IPVc":{"SerNr":"1009","@sequence":"5115"}}}`, "1009", true},
		{"empty list", 200, `{"data":{"@register":"IPVc","@sequence":"5121","IPVc":[]}}`, "", true},
		{"no IPVc key", 200, `{"data":{"@register":"IPVc","@sequence":"5121"}}`, "", false},
		{"other shape", 200, `{"data":{"deleted":[{"id":"1009"}]}}`, "", false},
		{"list of strings", 200, `{"data":{"IPVc":["1009"]}}`, "", false},
		{"record without SerNr", 200, `{"data":{"IPVc":[{"@sequence":"5115"}]}}`, "", false},
		{"record without sequence", 200, `{"data":{"IPVc":[{"SerNr":"1009"}]}}`, "", false},
		{"record below the cursor", 200, `{"data":{"IPVc":[{"SerNr":"1009","@sequence":"40"}]}}`, "", false},
		// A register ignoring deletes_after lists live receipts — the very ones
		// the update read just returned.
		{"live receipt listed", 200, `{"data":{"IPVc":[{"SerNr":"1001","@sequence":"5101","TransDate":"2026-09-01","rows":[]}]}}`, "", false},
		{"not json", 200, `{"data":"nope"}`, "", false},
		{"refused 400", 400, `{"error":{"@code":"1","@description":"no"}}`, "", false},
		{"refused on 200", 200, `{"error":{"@code":"1","@description":"no"}}`, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, _ := newEBFake(t, func(n int, w ebWire) (int, string) {
				if strings.Contains(w.Query, "deletes_after") {
					return c.status, c.body
				}
				return 200, live
			})
			got, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{SinceSequence: "5100", WithDeletions: true})
			if err != nil {
				t.Fatalf("ListPaymentChanges: %v", err)
			}
			if strings.Join(got.Deleted, ",") != c.want || got.DeletedComplete != c.complete {
				t.Fatalf("deleted = %v complete=%v, want [%s] complete=%v", got.Deleted, got.DeletedComplete, c.want, c.complete)
			}
			if !c.complete && len(got.Deleted) != 0 {
				t.Fatalf("an unproven answer must name no ids: %v", got.Deleted)
			}
		})
	}
}

// A deletion list that fills its page may continue: its ids stand, its
// completeness does not.
func TestEBListPaymentChangesDeletesAfterFullPage(t *testing.T) {
	p, _ := newEBFake(t, func(n int, w ebWire) (int, string) {
		if strings.Contains(w.Query, "deletes_after") {
			return 200, `{"data":{"IPVc":[{"SerNr":"9","@sequence":"120"},{"SerNr":"8","@sequence":"121"}]}}`
		}
		return 200, ebPage("130")
	})
	p.pageSize = 2
	got, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{SinceSequence: "100", WithDeletions: true})
	if err != nil {
		t.Fatalf("ListPaymentChanges: %v", err)
	}
	if strings.Join(got.Deleted, ",") != "9,8" || got.DeletedComplete {
		t.Fatalf("deleted = %v complete=%v, want [9 8] incomplete", got.Deleted, got.DeletedComplete)
	}
}

func TestEBListPaymentChangesDeletesAfterServerError(t *testing.T) {
	p, _ := newEBFake(t, func(n int, w ebWire) (int, string) {
		if strings.Contains(w.Query, "deletes_after") {
			return 502, `bad gateway`
		}
		return 200, ebPage("130")
	})
	_, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{SinceSequence: "100", WithDeletions: true})
	if err == nil || errors.Is(err, ErrChangeFeedUnsupported) {
		t.Fatalf("err = %v, want an ordinary error", err)
	}
}

// --- ReverseReceipt ---

const ebReverseOriginal = `{"data":{"@register":"IPVc","IPVc":[{"@sequence":"5106","SerNr":"1006","TransDate":"2026-09-06","PayMode":"P2",` +
	`"Comment":"KLUBIO-PMT-55","OKFlag":"1","PayCurCode":"EUR","Objects":"SPORT",` +
	`"rows":[{"@rownumber":"0","stp":"1","InvoiceNr":"260105","CustCode":"K5","RecVal":"90.00","BankVal":"90.00","PayDate":"2026-09-06","Objects":"TREENER","Comment":"x"},` +
	`{"@rownumber":"1","stp":"1","CUPNr":"2704","CustCode":"K5","RecVal":"30.00","PayDate":"2026-09-06"},` +
	`{"@rownumber":"2","stp":"2","CustCode":"K5","RecVal":"0.40","PayDate":"2026-09-06"}]}]}}`

func TestEBReverseReceiptWire(t *testing.T) {
	p, f := newEBFake(t, func(n int, w ebWire) (int, string) {
		if w.Method == "GET" {
			return 200, ebReverseOriginal
		}
		return 200, ebCreated("1042")
	})
	got, err := p.ReverseReceipt(ebCtx, ReverseReceiptInput{ID: "1006", Date: ebDay(2026, 10, 14), Document: "KLUBIO-REV-1006"})
	if err != nil {
		t.Fatalf("ReverseReceipt: %v", err)
	}
	assertEBWire(t, f,
		`GET IPVc/1006`,
		`POST IPVc set_field.OKFlag=1&set_field.Objects=SPORT&set_field.PayCurCode=EUR&set_field.PayMode=P2&set_field.TransDate=2026-10-14&set_field.Comment=KLUBIO-REV-1006`+
			`&set_row_field.0.BankVal=-90&set_row_field.0.CustCode=K5&set_row_field.0.InvoiceNr=260105&set_row_field.0.Objects=TREENER&set_row_field.0.PayDate=2026-10-14&set_row_field.0.RecVal=-90&set_row_field.0.stp=1`+
			`&set_row_field.1.CUPNr=2704&set_row_field.1.CustCode=K5&set_row_field.1.PayDate=2026-10-14&set_row_field.1.RecVal=-30&set_row_field.1.stp=1`+
			`&set_row_field.2.CustCode=K5&set_row_field.2.PayDate=2026-10-14&set_row_field.2.RecVal=-0.4&set_row_field.2.stp=2`,
	)
	if got.ID != "1042" || got.Number != "1042" || !got.Amount.Equal(d("-120.40")) || got.InvoiceID != "260105" || got.CustomerID != "K5" {
		t.Fatalf("reversal = %+v", got)
	}
}

func TestEBReverseReceiptKeepsTheOriginalsDate(t *testing.T) {
	p, f := newEBFake(t, func(n int, w ebWire) (int, string) {
		if w.Method == "GET" {
			return 200, ebPage("1", ebRec("1001", "2026-09-01", "5101", "1", ebInvRow("260100", "K1", "45.00")))
		}
		return 200, `{"data":{}}`
	})
	got, err := p.ReverseReceipt(ebCtx, ReverseReceiptInput{ID: "1001"})
	if err != nil {
		t.Fatalf("ReverseReceipt: %v", err)
	}
	assertEBWire(t, f,
		`GET IPVc/1001`,
		`POST IPVc set_field.OKFlag=1&set_field.PayCurCode=EUR&set_field.PayMode=P2&set_field.TransDate=2026-09-01&set_row_field.0.CustCode=K1&set_row_field.0.InvoiceNr=260100&set_row_field.0.PayDate=2026-09-01&set_row_field.0.RecVal=-45&set_row_field.0.stp=1`,
	)
	// No SerNr in the answer is not a failure; the Document finds it later.
	if got.ID != "" || !got.Amount.Equal(d("-45")) || got.InvoiceID != "260100" {
		t.Fatalf("reversal = %+v", got)
	}
}

func TestEBReverseReceiptRefusesBeforeWriting(t *testing.T) {
	cases := []struct {
		name string
		body string
		is   error
	}{
		{"draft", ebPage("1", ebRec("1001", "2026-09-01", "1", "0", ebInvRow("260100", "K1", "45"))), ErrReceiptNotReversible},
		{"void", strings.Replace(ebPage("1", ebRec("1001", "2026-09-01", "1", "1", ebInvRow("260100", "K1", "45"))), `"OKFlag"`, `"Invalid":"1","OKFlag"`, 1), ErrReceiptNotReversible},
		{"no rows", ebPage("1", ebRec("1001", "2026-09-01", "1", "1")), ErrReceiptNotReversible},
		{"unreadable amount", ebPage("1", ebRec("1001", "2026-09-01", "1", "1", ebInvRow("260100", "K1", "x"))), ErrReceiptNotReversible},
		{"gone", ebPage("1"), ErrNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, f := newEBFake(t, func(int, ebWire) (int, string) { return 200, c.body })
			if _, err := p.ReverseReceipt(ebCtx, ReverseReceiptInput{ID: "1001", Document: "KLUBIO-REV-1001"}); !errors.Is(err, c.is) {
				t.Fatalf("err = %v, want %v", err, c.is)
			}
			assertEBWire(t, f, `GET IPVc/1001`)
		})
	}
	p, f := newEBFake(t, nil)
	if _, err := p.ReverseReceipt(ebCtx, ReverseReceiptInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("no id: %v", err)
	}
	assertEBWire(t, f)
}

func TestEBReverseReceiptPostRejected(t *testing.T) {
	p, _ := newEBFake(t, func(n int, w ebWire) (int, string) {
		if w.Method == "GET" {
			return 200, ebReverseOriginal
		}
		return 200, `{"error":{"@code":"20060","@description":"Summa on suurem kui arve summa 0"}}`
	})
	if got, err := p.ReverseReceipt(ebCtx, ReverseReceiptInput{ID: "1006"}); err == nil {
		t.Fatalf("ReverseReceipt = %+v, want EB's refusal", got)
	}
}

// --- Decoding ---

func TestMapEBReceiptDecoding(t *testing.T) {
	row := func(stp, inv, cup, cust, val, comment string) excellentbooks.ReceiptRow {
		return excellentbooks.ReceiptRow{Stp: stp, InvoiceNr: inv, CUPNr: cup, CustCode: cust, RecVal: val, Comment: comment}
	}
	type want struct {
		rows      string // type:doc:amount, comma-separated
		amount    string
		advance   string
		sources   string
		customer  string
		extras    bool
		draft     bool
		void      bool
		decodeErr bool
	}
	cases := []struct {
		name string
		r    excellentbooks.Receipt
		w    want
	}{
		{"cash on an invoice", excellentbooks.Receipt{OKFlag: "1", Rows: []excellentbooks.ReceiptRow{row("1", "260100", "", "K1", "45.00", "arve")}},
			want{rows: "CLIENT_INVOICE:260100:45", amount: "45", advance: "0", customer: "K1"}},
		{"advance applied", excellentbooks.Receipt{OKFlag: "1", Rows: []excellentbooks.ReceiptRow{row("1", "260101", "", "K1", "75", ""), row("1", "", "90000006", "K1", "-75", "")}},
			want{rows: "CLIENT_INVOICE:260101:75,PREPAYMENT_PAYMENT:90000006:-75", amount: "0", advance: "0", sources: "90000006", customer: "K1"}},
		{"paid money freed", excellentbooks.Receipt{OKFlag: "1", Rows: []excellentbooks.ReceiptRow{row("1", "260102", "", "K2", "-120", ""), row("1", "", "90000000", "K2", "120", "")}},
			want{rows: "CLIENT_INVOICE:260102:-120,PREPAYMENT_PAYMENT:90000000:120", amount: "0", advance: "120", customer: "K2"}},
		{"advance parked", excellentbooks.Receipt{OKFlag: "1", Rows: []excellentbooks.ReceiptRow{row("1", "", "KLB-PP-3", "K3", "40", "")}},
			want{rows: "PREPAYMENT_PAYMENT:KLB-PP-3:40", amount: "40", advance: "40", customer: "K3"}},
		{"two customers", excellentbooks.Receipt{OKFlag: "1", Rows: []excellentbooks.ReceiptRow{row("1", "1", "", "K1", "10", ""), row("1", "2", "", "K2", "5", "")}},
			want{rows: "CLIENT_INVOICE:1:10,CLIENT_INVOICE:2:5", amount: "15", advance: "0"}},
		{"extra row", excellentbooks.Receipt{OKFlag: "1", Rows: []excellentbooks.ReceiptRow{row("1", "1", "", "K7", "20", ""), row("2", "", "", "K7", "0.40", "pangatasu")}},
			want{rows: "CLIENT_INVOICE:1:20", amount: "20", advance: "0", customer: "K7", extras: true}},
		{"draft", excellentbooks.Receipt{OKFlag: "0", Rows: []excellentbooks.ReceiptRow{row("1", "1", "", "K1", "15", "")}},
			want{rows: "CLIENT_INVOICE:1:15", amount: "15", advance: "0", customer: "K1", draft: true}},
		{"no OKFlag is a draft", excellentbooks.Receipt{Rows: []excellentbooks.ReceiptRow{row("1", "1", "", "K1", "15", "")}},
			want{rows: "CLIENT_INVOICE:1:15", amount: "15", advance: "0", customer: "K1", draft: true}},
		{"void", excellentbooks.Receipt{OKFlag: "1", Invalid: "1", Rows: []excellentbooks.ReceiptRow{row("1", "1", "", "K1", "15", "")}},
			want{rows: "CLIENT_INVOICE:1:15", amount: "15", advance: "0", customer: "K1", void: true}},
		{"Invalid 0 is live", excellentbooks.Receipt{OKFlag: "1", Invalid: "0", Rows: []excellentbooks.ReceiptRow{row("1", "1", "", "K1", "15", "")}},
			want{rows: "CLIENT_INVOICE:1:15", amount: "15", advance: "0", customer: "K1"}},
		{"Invalid of an odd shape", excellentbooks.Receipt{OKFlag: "1", Invalid: `{"x":1}`, Rows: []excellentbooks.ReceiptRow{row("1", "1", "", "K1", "15", "")}},
			want{rows: "CLIENT_INVOICE:1:15", amount: "15", advance: "0", customer: "K1", decodeErr: true}},
		{"row with both", excellentbooks.Receipt{OKFlag: "1", Rows: []excellentbooks.ReceiptRow{row("1", "1", "C1", "K1", "15", ""), row("1", "2", "", "K1", "5", "")}},
			want{rows: "CLIENT_INVOICE:2:5", amount: "5", advance: "0", customer: "K1", decodeErr: true}},
		{"row with neither", excellentbooks.Receipt{OKFlag: "1", Rows: []excellentbooks.ReceiptRow{row("1", "", "", "K1", "15", "")}},
			want{rows: "", amount: "0", advance: "0", customer: "K1", decodeErr: true}},
		{"unreadable amount", excellentbooks.Receipt{OKFlag: "1", Rows: []excellentbooks.ReceiptRow{row("1", "1", "", "K1", "", "")}},
			want{rows: "", amount: "0", advance: "0", customer: "K1", decodeErr: true}},
		{"no rows", excellentbooks.Receipt{OKFlag: "1"}, want{rows: "", amount: "0", advance: "0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.r.SerNr, c.r.TransDate, c.r.PayMode, c.r.PayCurCode, c.r.Comment, c.r.Sequence = "77", "2026-09-01", "P2", "EUR", "KLUBIO-PMT-9", "5101"
			r := mapEBReceipt(c.r)
			rows := make([]string, len(r.Rows))
			for i, rr := range r.Rows {
				rows[i] = rr.Type + ":" + rr.DocID + ":" + rr.Amount.String()
			}
			got := want{
				rows: strings.Join(rows, ","), amount: r.Amount.String(), advance: r.AdvanceAmount.String(),
				sources: strings.Join(r.AdvanceSourceIDs, ","), customer: r.CustomerID,
				extras: r.HasExtras, draft: r.Draft, void: r.Void, decodeErr: r.DecodeError != "",
			}
			if got != c.w {
				t.Fatalf("decoded\n got  %+v\n want %+v\n (DecodeError %q)", got, c.w, r.DecodeError)
			}
			if r.ID != "77" || r.Number != "77" || r.Document != "KLUBIO-PMT-9" || r.AccountName != "P2" || r.Currency != "EUR" ||
				r.Sequence != "5101" || r.Date != ebDay(2026, 9, 1) || r.CreatedBy != "" || r.Comment != "" {
				t.Fatalf("header decoded wrong: %+v", r)
			}
		})
	}
}

// The comment of a row reaches Description, and the generic helpers work on
// an EB receipt unchanged.
func TestMapEBReceiptWithGenericHelpers(t *testing.T) {
	r := mapEBReceipt(excellentbooks.Receipt{SerNr: "1", OKFlag: "1", Rows: []excellentbooks.ReceiptRow{
		{Stp: "1", InvoiceNr: "260100", CustCode: "K1", RecVal: "45.00", Comment: "arve 260100"},
	}})
	if r.Rows[0].Description != "arve 260100" || !r.AmountOn("260100").Equal(d("45")) || !r.TouchesOnly("260100") {
		t.Fatalf("receipt = %+v", r)
	}
	two := mapEBReceipt(excellentbooks.Receipt{SerNr: "2", OKFlag: "1", Rows: []excellentbooks.ReceiptRow{
		{Stp: "1", InvoiceNr: "260100", CustCode: "K1", RecVal: "45"}, {Stp: "2", CustCode: "K1", RecVal: "0"},
	}})
	if two.TouchesOnly("260100") {
		t.Fatal("a receipt with an extra row must not touch only its invoice")
	}
}

// A field of an unexpected JSON type on a receipt must never fail the decode
// of a list every unlisted club reads (ListPayments).
func TestEBInvalidOfAnyTypeDecodes(t *testing.T) {
	body := ebPage("1",
		strings.Replace(ebRec("1", "2026-09-01", "1", "1", ebInvRow("1", "K1", "1")), `"OKFlag"`, `"Invalid":0,"OKFlag"`, 1),
		strings.Replace(ebRec("2", "2026-09-01", "2", "1", ebInvRow("2", "K1", "2")), `"OKFlag"`, `"Invalid":true,"OKFlag"`, 1),
		strings.Replace(ebRec("3", "2026-09-01", "3", "1", ebInvRow("3", "K1", "3")), `"OKFlag"`, `"Invalid":null,"OKFlag"`, 1),
		strings.Replace(ebRec("4", "2026-09-01", "4", "1", ebInvRow("4", "K1", "4")), `"OKFlag"`, `"Invalid":{"a":1},"OKFlag"`, 1),
	)
	p, _ := newEBFake(t, func(int, ebWire) (int, string) { return 200, body })
	if ps, err := p.ListPayments(ebCtx, ListPaymentsInput{}); err != nil || len(ps) != 4 {
		t.Fatalf("ListPayments = %d, %v", len(ps), err)
	}
	got, err := p.ListPaymentChanges(ebCtx, PaymentChangesInput{Since: ebDay(2026, 1, 1)})
	if err != nil {
		t.Fatalf("ListPaymentChanges: %v", err)
	}
	var flags []string
	for _, r := range got.Receipts {
		flags = append(flags, fmt.Sprintf("%s:%v:%v", r.ID, r.Void, r.DecodeError != ""))
	}
	if s := strings.Join(flags, " "); s != "1:false:false 2:true:false 3:false:false 4:false:true" {
		t.Fatalf("void/decode flags = %s", s)
	}
}

// The advance receipts klubio writes itself — an overpayment parked, a paid
// invoice freed before its credit note — can carry klubio's mark in the
// header Comment, after PayMode like every tagged receipt. Without Document
// the request is the golden one (TestEBCreatePrepaymentWireGolden,
// TestEBUnallocateToPrepaymentWireGolden).
func TestEBPrepaymentDocumentWire(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) { return 200, ebCreated("300009") })
	if _, err := p.CreatePrepayment(ebCtx, CreatePrepaymentInput{
		CustomerCode: "K7224", PrepaymentNo: "600000001", Amount: d("5"), Currency: "EUR",
		PaymentDate: ebDay(2026, 10, 21), BankID: "K", Comment: "Overpayment on invoice EBT000002",
		Document: "KLUBIO-PARK-600000001",
	}); err != nil {
		t.Fatalf("CreatePrepayment: %v", err)
	}
	if _, err := p.UnallocateToPrepayment(ebCtx, UnallocateToPrepaymentInput{
		CustomerCode: "K7224", InvoiceNo: "260064", PrepaymentNo: "600000002", Amount: d("55"), Currency: "EUR",
		PaymentDate: ebDay(2026, 10, 21), BankID: "P", Comment: "Credit of paid invoice EBT000001",
		Document: "KLUBIO-FREE-600000002",
	}); err != nil {
		t.Fatalf("UnallocateToPrepayment: %v", err)
	}
	assertEBWire(t, f,
		`POST IPVc set_field.OKFlag=1&set_field.PayCurCode=EUR&set_field.PayMode=K&set_field.TransDate=2026-10-21&set_field.Comment=KLUBIO-PARK-600000001&set_row_field.0.CUPNr=600000001&set_row_field.0.Comment=Overpayment%20on%20invoice%20EBT000002&set_row_field.0.CustCode=K7224&set_row_field.0.PayDate=2026-10-21&set_row_field.0.RecVal=5&set_row_field.0.stp=1`,
		`POST IPVc set_field.OKFlag=1&set_field.PayCurCode=EUR&set_field.PayMode=P&set_field.TransDate=2026-10-21&set_field.Comment=KLUBIO-FREE-600000002&set_row_field.0.Comment=Credit%20of%20paid%20invoice%20EBT000001&set_row_field.0.CustCode=K7224&set_row_field.0.InvoiceNr=260064&set_row_field.0.PayDate=2026-10-21&set_row_field.0.RecVal=-55&set_row_field.0.stp=1&set_row_field.1.CUPNr=600000002&set_row_field.1.CustCode=K7224&set_row_field.1.PayDate=2026-10-21&set_row_field.1.RecVal=55&set_row_field.1.stp=1`,
	)
}
