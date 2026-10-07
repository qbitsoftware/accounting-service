package accounting

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func saReceiptInput() CreatePaymentInput {
	return CreatePaymentInput{
		CustomerName: "Mari Maasikas",
		CustomerCode: "cli-9",
		InvoiceNo:    "2026-0042",
		PaymentDate:  saDay(2026, 9, 3),
		Amount:       d("25.50"),
		Currency:     "EUR",
		BankID:       "LHV klubio",
	}
}

func TestSACreateReceiptResolvedInvoiceSkipsFind(t *testing.T) {
	p, f := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"paymentId":"pay-77","number":"12","amount":25.5}`
	})
	in := saReceiptInput()
	in.Document = "KLUBIO-PMT-501"
	in.ResolvedInvoice = &ResolvedProviderInvoice{ID: "inv-1", CustomerID: "cli-9", Currency: "EUR"}
	got, err := p.CreateReceipt(saCtx, in)
	if err != nil {
		t.Fatalf("CreateReceipt: %v", err)
	}
	if got.ID != "pay-77" || got.InvoiceID != "inv-1" || got.CustomerID != "cli-9" {
		t.Errorf("created = %+v", got)
	}
	// Exactly one request: the held read already resolved the invoice.
	assertSAWire(t, f, saTestKey(t),
		`POST purchasesales/payments:add?apikey=KEY {"date":"03.09.2026","document":"KLUBIO-PMT-501","partnerType":"CLIENT","clientId":"cli-9","accountType":"BANK","accountName":"LHV klubio","currency":"EUR","amount":"25.5","rows":[{"amount":"25.5","type":"CLIENT_INVOICE","id":"inv-1"}]}`,
	)
}

func TestSACreateReceiptReturnsPaymentID(t *testing.T) {
	p, f := newSAFake(t, func(n int, w saWire) (int, string) {
		if strings.HasPrefix(w.Path, "purchasesales/clientinvoices:get") {
			return 200, saInvoiceByNumber
		}
		return 200, `{"paymentId":"pay-78","number":13,"amount":25.5}`
	})
	in := saReceiptInput()
	in.ExpectInvoiceID = "inv-1"
	got, err := p.CreateReceipt(saCtx, in)
	if err != nil {
		t.Fatalf("CreateReceipt: %v", err)
	}
	if got.ID != "pay-78" || got.Number != "13" || !got.Amount.Equal(d("25.5")) || got.InvoiceID != "inv-1" || got.CustomerID != "cli-9" {
		t.Errorf("created = %+v", got)
	}
	// Without a document or comment the body is CreatePayment's, byte for byte.
	assertSAWire(t, f, saTestKey(t),
		`GET purchasesales/clientinvoices:get?apikey=KEY&dateFrom=01.01.2000&fetchRows=true&invoiceNumber=2026-0042&pageNumber=1`,
		`POST purchasesales/payments:add?apikey=KEY {"date":"03.09.2026","partnerType":"CLIENT","clientId":"cli-9","accountType":"BANK","accountName":"LHV klubio","currency":"EUR","amount":"25.5","rows":[{"amount":"25.5","type":"CLIENT_INVOICE","id":"inv-1"}]}`,
	)
}

func TestSACreateReceiptEmptyIDIsNotError(t *testing.T) {
	// A receipt SA booked but whose id did not come back still exists: the
	// caller recognises it later by its document. An empty body included.
	for name, body := range map[string]string{"no id": `{"number":"12"}`, "empty body": ``} {
		t.Run(name, func(t *testing.T) {
			p, _ := newSAFake(t, func(int, saWire) (int, string) { return 200, body })
			in := saReceiptInput()
			in.ResolvedInvoice = &ResolvedProviderInvoice{ID: "inv-1", CustomerID: "cli-9"}
			got, err := p.CreateReceipt(saCtx, in)
			if err != nil {
				t.Fatalf("an empty id must not be an error: %v", err)
			}
			if got.ID != "" || !got.Amount.Equal(d("25.5")) {
				t.Errorf("created = %+v", got)
			}
		})
	}
}

func TestSACreateReceiptCashDocument(t *testing.T) {
	p, f := newSAFake(t, func(int, saWire) (int, string) { return 200, `{"paymentId":"cash-1","amount":15}` })
	in := saReceiptInput()
	in.Amount = d("15")
	in.BankID = "Kassa"
	in.AccountKind = PaymentAccountCash
	in.Document = "KLUBIO-PMT-502"
	in.Comment = "Sularaha treeningul"
	in.ResolvedInvoice = &ResolvedProviderInvoice{ID: "inv-3", CustomerID: "cli-7", Currency: "EUR"}
	if _, err := p.CreateReceipt(saCtx, in); err != nil {
		t.Fatalf("CreateReceipt: %v", err)
	}
	assertSAWire(t, f, saTestKey(t),
		`POST purchasesales/payments:add?apikey=KEY {"date":"03.09.2026","document":"KLUBIO-PMT-502","partnerType":"CLIENT","clientId":"cli-7","accountType":"CASH","accountName":"Kassa","currency":"EUR","amount":"15","comment":"Sularaha treeningul","rows":[{"amount":"15","type":"CLIENT_INVOICE","id":"inv-3"}]}`,
	)
}

func TestSACreateReceiptExpectInvoiceIDRefusesBeforeAdd(t *testing.T) {
	p, f := newSAFake(t, func(int, saWire) (int, string) { return 200, saInvoiceByNumber })
	in := saReceiptInput()
	in.ExpectInvoiceID = "inv-OTHER"
	_, err := p.CreateReceipt(saCtx, in)
	if !errors.Is(err, ErrProviderInvoiceMismatch) {
		t.Fatalf("want ErrProviderInvoiceMismatch, got %v", err)
	}
	if w := f.wire(); len(w) != 1 || w[0].Method != "GET" {
		t.Fatalf("only the lookup may be sent, got %v", w)
	}

	// The same guard holds when a resolved invoice contradicts the expectation.
	p2, f2 := newSAFake(t, nil)
	in.ResolvedInvoice = &ResolvedProviderInvoice{ID: "inv-1", CustomerID: "cli-9"}
	if _, err := p2.CreateReceipt(saCtx, in); !errors.Is(err, ErrProviderInvoiceMismatch) || len(f2.wire()) != 0 {
		t.Fatalf("resolved/expected mismatch: err=%v requests=%d", err, len(f2.wire()))
	}
}

func TestSAUnknownAccountKindRejectedBeforeHTTP(t *testing.T) {
	p, f := newSAFake(t, nil)
	in := saReceiptInput()
	in.AccountKind = "card"
	if _, err := p.CreateReceipt(saCtx, in); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
	in.AccountKind = PaymentAccountDefault
	in.ResolvedInvoice = &ResolvedProviderInvoice{ID: "inv-1"} // no customer id
	if _, err := p.CreateReceipt(saCtx, in); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("a resolved invoice without its customer must be refused, got %v", err)
	}
	if n := len(f.wire()); n != 0 {
		t.Fatalf("nothing may be sent, %d requests", n)
	}
}

func TestSACreateReceiptCurrencyGuard(t *testing.T) {
	p, f := newSAFake(t, nil)
	in := saReceiptInput()
	in.ResolvedInvoice = &ResolvedProviderInvoice{ID: "inv-1", CustomerID: "cli-9", Currency: "USD"}
	if _, err := p.CreateReceipt(saCtx, in); !errors.Is(err, ErrInvalidInput) || len(f.wire()) != 0 {
		t.Fatalf("currency mismatch: err=%v requests=%d", err, len(f.wire()))
	}
}

func TestSACreateReceiptRateLimitSurfaces(t *testing.T) {
	p, f := newSAFake(t, func(int, saWire) (int, string) { return 503, `{"error":"Rate Limit Exceeded"}` })
	in := saReceiptInput()
	in.ResolvedInvoice = &ResolvedProviderInvoice{ID: "inv-1", CustomerID: "cli-9"}
	_, err := p.CreateReceipt(saCtx, in)
	if !IsRateLimit(err) {
		t.Fatalf("want a rate-limit error, got %v", err)
	}
	if n := len(f.wire()); n != 4 {
		t.Fatalf("one attempt plus three retries, got %d", n)
	}
}

func TestSAApplyPrepaymentDocumentOptIn(t *testing.T) {
	p, f := newSAFake(t, func(n int, w saWire) (int, string) {
		if strings.HasPrefix(w.Path, "purchasesales/clientinvoices:get") {
			return 200, saInvoiceByNumber
		}
		return 200, `{"paymentId":"app-1"}`
	})
	in := ApplyPrepaymentInput{CustomerCode: "cli-9", InvoiceNo: "2026-0042", PrepaymentNo: "adv-1",
		Amount: d("20"), Currency: "EUR", PaymentDate: saDay(2026, 9, 5), BankID: "LHV klubio", Document: "KLUBIO-APPLY-77"}
	if err := p.ApplyPrepayment(saCtx, in); err != nil {
		t.Fatal(err)
	}
	assertSAWire(t, f, saTestKey(t),
		`GET purchasesales/clientinvoices:get?apikey=KEY&dateFrom=01.01.2000&fetchRows=true&invoiceNumber=2026-0042&pageNumber=1`,
		`POST purchasesales/payments:add?apikey=KEY {"date":"05.09.2026","document":"KLUBIO-APPLY-77","partnerType":"CLIENT","clientId":"cli-9","accountType":"BANK","accountName":"LHV klubio","currency":"EUR","amount":"0","rows":[{"amount":"20","type":"CLIENT_INVOICE","id":"inv-1"},{"amount":"20","type":"PREPAYMENT_PAYMENT","id":"adv-1"}]}`,
	)
	// Zero Document is the v0.14.5 request: TestSAPrepaymentApplyWireGolden.
}

// ApplyPrepaymentReceipt sends exactly ApplyPrepayment's request and returns
// the application's paymentId; an answer without an id is not a failure.
func TestSAApplyPrepaymentReceiptReturnsID(t *testing.T) {
	in := ApplyPrepaymentInput{CustomerCode: "cli-9", InvoiceNo: "2026-0042", PrepaymentNo: "adv-1",
		Amount: d("20"), Currency: "EUR", PaymentDate: saDay(2026, 9, 5), BankID: "LHV klubio", Document: "KLUBIO-APPLY-77"}
	respond := func(body string) func(int, saWire) (int, string) {
		return func(n int, w saWire) (int, string) {
			if strings.HasPrefix(w.Path, "purchasesales/clientinvoices:get") {
				return 200, saInvoiceByNumber
			}
			return 200, body
		}
	}

	plain, pf := newSAFake(t, respond(`{"paymentId":"app-1","number":14}`))
	if err := plain.ApplyPrepayment(saCtx, in); err != nil {
		t.Fatal(err)
	}
	p, f := newSAFake(t, respond(`{"paymentId":"app-1","number":14}`))
	got, err := p.ApplyPrepaymentReceipt(saCtx, in)
	if err != nil {
		t.Fatalf("ApplyPrepaymentReceipt: %v", err)
	}
	if got.ID != "app-1" || got.Number != "14" || got.InvoiceID != "inv-1" || got.CustomerID != "cli-9" {
		t.Errorf("created = %+v", got)
	}
	a, b := pf.wire(), f.wire()
	if len(a) != len(b) {
		t.Fatalf("requests: ApplyPrepayment %d, ApplyPrepaymentReceipt %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Method != b[i].Method || a[i].Path != b[i].Path || a[i].Body != b[i].Body {
			t.Errorf("request %d differs:\n  apply   %s\n  receipt %s", i, a[i], b[i])
		}
	}

	empty, _ := newSAFake(t, respond(``))
	got, err = empty.ApplyPrepaymentReceipt(saCtx, in)
	if err != nil || got == nil || got.ID != "" {
		t.Errorf("empty answer: %+v, %v (want an empty id, no error)", got, err)
	}

	bad := in
	bad.PrepaymentNo = ""
	if _, err := p.ApplyPrepaymentReceipt(saCtx, bad); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("missing advance id: %v, want ErrInvalidInput", err)
	}
}

func TestSAInvoiceDocTypeMapped(t *testing.T) {
	p, _ := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"clientInvoices":[{"id":"cre-2","clientId":"cli-8","type":"CRE","baseForCreditInvoiceId":"inv-2",` +
			`"invoiceNumber":"2026-0050","totalAmount":-30,"outstandingAmount":0}],"hasMoreEntries":false}`
	})
	inv, err := p.GetInvoice(saCtx, "cre-2")
	if err != nil {
		t.Fatal(err)
	}
	if inv.DocType != "CRE" || inv.BaseForCreditInvoiceID != "inv-2" || inv.Payments != nil {
		t.Fatalf("invoice = %+v", inv)
	}
}

func TestSAListPaymentChanges(t *testing.T) {
	p, f := newSAFake(t, func(n int, w saWire) (int, string) {
		if strings.Contains(w.Query, "pageNumber=2") {
			return 200, saFeedPage2
		}
		return 200, saFeedPage1
	})
	since := time.Date(2026, 9, 1, 6, 0, 5, 0, time.UTC)
	ch, err := p.ListPaymentChanges(saCtx, PaymentChangesInput{Since: since, Until: saDay(2027, 1, 5), Precise: true})
	if err != nil {
		t.Fatalf("ListPaymentChanges: %v", err)
	}
	assertSAWire(t, f, saTestKey(t),
		`GET purchasesales/payments:get?apikey=KEY&dateFrom=01.09.2026_09%3A00%3A05&dateTo=05.01.2027&dateType=modifydate&fetchComments=true&fetchRows=true&pageNumber=1`,
		`GET purchasesales/payments:get?apikey=KEY&dateFrom=01.09.2026_09%3A00%3A05&dateTo=05.01.2027&dateType=modifydate&fetchComments=true&fetchRows=true&pageNumber=2`,
	)
	if strings.Join(ch.Deleted, ",") != "gone-1,gone-2" || !ch.DeletedComplete {
		t.Errorf("deleted = %v complete=%v", ch.Deleted, ch.DeletedComplete)
	}
	// The projection is exactly what ListPaymentsSince decodes from the same feed.
	want := "adv-1|5|2026-09-01|50|EUR|customer|cli-9|Mari Maasikas|LHV klubio|LHV klubio|\n" +
		"app-1|6|2026-09-02|0|EUR|customer|cli-9|Mari Maasikas|LHV klubio|LHV klubio|[inv-1 \"\" 20 fp=0]\n" +
		"net-1|7|2026-09-03|60|EUR|customer|cli-8|Jaan Tamm|Tasaarveldus|Tasaarveldus|[inv-2 \"\" 30 fp=0][cre-2 \"\" 30 fp=0]\n" +
		"cash-1|17|2026-09-04|15|EUR|customer|cli-7|Kati Kask|Kassa|Kassa|[inv-3 \"\" 15 fp=0]\n" +
		"ven-1|8|2026-09-05|300|EUR|vendor|ven-9|Saalirent OÜ|LHV klubio|LHV klubio|[vi-1 \"\" 300 fp=0]\n"
	if r := renderSAPayments(ch.Payments); r != want {
		t.Fatalf("projection differs from ListPaymentsSince\n got:\n%s\n want:\n%s", r, want)
	}

	by := map[string]Receipt{}
	for _, r := range ch.Receipts {
		by[r.ID] = r
	}
	if len(ch.Receipts) != 5 {
		t.Fatalf("receipts = %d", len(ch.Receipts))
	}
	adv := by["adv-1"]
	if !adv.AdvanceAmount.Equal(d("50")) || adv.Document != "KLB-PP-3" || len(adv.Rows) != 0 || adv.CustomerID != "cli-9" {
		t.Errorf("advance park = %+v", adv)
	}
	app := by["app-1"]
	if strings.Join(app.AdvanceSourceIDs, ",") != "adv-1" || len(app.Rows) != 2 || !app.AmountOn("inv-1").Equal(d("20")) || app.TouchesOnly("inv-1") {
		t.Errorf("application = %+v", app)
	}
	// A draw is a NEGATIVE PREPAYMENT_PAYMENT row (live 07.10.2026): the rows
	// sum to the application's amount, 0. The SDK passes the sign through.
	if len(app.Rows) != 2 || app.Rows[0].Type != "PREPAYMENT_PAYMENT" || !app.Rows[0].Amount.Equal(d("-20")) || !app.Amount.IsZero() || !app.CreatedThroughAPI() {
		t.Errorf("application rows = %+v amount=%s createdBy=%q", app.Rows, app.Amount, app.CreatedBy)
	}
	net := by["net-1"]
	if !net.AmountOn("inv-2").Equal(d("30")) || !net.AmountOn("cre-2").Equal(d("30")) || net.TouchesOnly("inv-2") {
		t.Errorf("netting = %+v", net)
	}
	cash := by["cash-1"]
	if cash.AccountType != "CASH" || cash.AccountName != "Kassa" || cash.Number != "17" || cash.OrderNumber != "3" ||
		!cash.TouchesOnly("inv-3") || cash.Date != saDay(2026, 9, 4) {
		t.Errorf("cash = %+v", cash)
	}
	if v := by["ven-1"]; v.PartnerType != "VENDOR" || v.CustomerID != "" {
		t.Errorf("vendor = %+v", v)
	}

	// Without Precise the date-only format is sent; ClientsOnly narrows to CLIENT.
	p2, f2 := newSAFake(t, func(int, saWire) (int, string) { return 200, `{"payments":[],"deleted":[],"hasMoreEntries":false}` })
	if _, err := p2.ListPaymentChanges(saCtx, PaymentChangesInput{Since: saDay(2026, 7, 9), Until: saDay(2027, 1, 5), ClientsOnly: true}); err != nil {
		t.Fatal(err)
	}
	assertSAWire(t, f2, saTestKey(t),
		`GET purchasesales/payments:get?apikey=KEY&dateFrom=09.07.2026&dateTo=05.01.2027&dateType=modifydate&fetchComments=true&fetchRows=true&pageNumber=1&partnerType=CLIENT`,
	)
	// Late evening UTC is already tomorrow in Tallinn. A cursor read sends
	// Tallinn's date as dateTo so the newest changes stay inside the window; the
	// date-only read keeps ListPaymentsSince's formatting byte for byte.
	lateUTC := time.Date(2026, 9, 1, 22, 30, 0, 0, time.UTC)
	p3, f3 := newSAFake(t, func(int, saWire) (int, string) { return 200, `{"payments":[],"deleted":[],"hasMoreEntries":false}` })
	if _, err := p3.ListPaymentChanges(saCtx, PaymentChangesInput{Since: since, Until: lateUTC, Precise: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := p3.ListPaymentChanges(saCtx, PaymentChangesInput{Since: saDay(2026, 7, 9), Until: lateUTC}); err != nil {
		t.Fatal(err)
	}
	if _, err := p3.ListPaymentsSince(saCtx, saDay(2026, 7, 9), lateUTC); err != nil {
		t.Fatal(err)
	}
	assertSAWire(t, f3, saTestKey(t),
		`GET purchasesales/payments:get?apikey=KEY&dateFrom=01.09.2026_09%3A00%3A05&dateTo=02.09.2026&dateType=modifydate&fetchComments=true&fetchRows=true&pageNumber=1`,
		`GET purchasesales/payments:get?apikey=KEY&dateFrom=09.07.2026&dateTo=01.09.2026&dateType=modifydate&fetchComments=true&fetchRows=true&pageNumber=1`,
		`GET purchasesales/payments:get?apikey=KEY&dateFrom=09.07.2026&dateTo=01.09.2026&dateType=modifydate&fetchRows=true&pageNumber=1`,
	)

	if _, err := p2.ListPaymentChanges(saCtx, PaymentChangesInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("a zero Since must be refused, got %v", err)
	}
}

func TestSAListPaymentChangesUnreadableRow(t *testing.T) {
	p, _ := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"payments":[{"id":"bad","amount":{"x":1}},{"id":"ok","date":"01.09.2026","partnerType":"CLIENT","amount":5}],"deleted":[],"hasMoreEntries":false}`
	})
	ch, err := p.ListPaymentChanges(saCtx, PaymentChangesInput{Since: saDay(2026, 9, 1)})
	if err != nil {
		t.Fatalf("an unreadable row must not fail the feed: %v", err)
	}
	if len(ch.Receipts) != 2 || ch.Receipts[0].DecodeError == "" || ch.Receipts[0].TouchesOnly("") {
		t.Fatalf("receipts = %+v", ch.Receipts)
	}
	if len(ch.Payments) != 1 || ch.Payments[0].ID != "ok" {
		t.Fatalf("an unreadable row has no projection: %+v", ch.Payments)
	}
}

func TestSAGetReceipt(t *testing.T) {
	p, f := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"payments":[{"id":"pay-77","document":"KLUBIO-PMT-501","comment":"c","partnerType":"CLIENT","client":{"id":"cli-9","name":"Mari"},` +
			`"accountType":"BANK","accountName":"LHV klubio","currency":"EUR","amount":25.5,"date":"03.09.2026",` +
			`"rows":[{"amount":25.5,"type":"CLIENT_INVOICE","id":"inv-1"}]}],"hasMoreEntries":false}`
	})
	r, err := p.GetReceipt(saCtx, "pay-77")
	if err != nil {
		t.Fatal(err)
	}
	if r.Document != "KLUBIO-PMT-501" || r.Comment != "c" || r.CustomerID != "cli-9" || r.CustomerName != "Mari" || !r.TouchesOnly("inv-1") {
		t.Errorf("receipt = %+v", r)
	}
	assertSAWire(t, f, saTestKey(t),
		`GET purchasesales/payments:get?apikey=KEY&dateFrom=01.01.2000&fetchComments=true&fetchRows=true&id=pay-77&pageNumber=1`,
	)

	gone, _ := newSAFake(t, func(int, saWire) (int, string) { return 200, `{"payments":[],"hasMoreEntries":false}` })
	if _, err := gone.GetReceipt(saCtx, "pay-77"); !IsNotFound(err) {
		t.Fatalf("an empty by-id answer is not-found, got %v", err)
	}
	wrong, _ := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"payments":[{"id":"x","amount":1}],"hasMoreEntries":false}`
	})
	if _, err := wrong.GetReceipt(saCtx, "pay-77"); !errors.Is(err, ErrIDFilterIgnored) || IsNotFound(err) {
		t.Fatalf("another payment in the answer must be ErrIDFilterIgnored, never not-found, got %v", err)
	}

	extras, _ := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"payments":[{"id":"pay-78","partnerType":"CLIENT","client":{"id":"cli-9"},"amount":25,` +
			`"rows":[{"amount":25,"type":"CLIENT_INVOICE","id":"inv-1"}],` +
			`"extras":[{"price":2,"quantity":1,"amount":2,"account":"8610"},{"price":-2,"quantity":1,"amount":-2,"account":"8610"}]}],"hasMoreEntries":false}`
	})
	r, err = extras.GetReceipt(saCtx, "pay-78")
	if err != nil {
		t.Fatal(err)
	}
	if !r.HasExtras || r.TouchesOnly("inv-1") {
		t.Errorf("extras summing to zero must still keep the receipt from reading as deletable: %+v", r)
	}
}

func TestSAListInvoiceReceipts(t *testing.T) {
	p, f := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"payments":[` +
			`{"id":"r1","partnerType":"CLIENT","client":{"id":"cli-9"},"amount":10,"rows":[{"amount":10,"type":"CLIENT_INVOICE","id":"inv-1"}]},` +
			`{"id":"r2","partnerType":"CLIENT","client":{"id":"cli-9"},"amount":15,"rows":[{"amount":15,"type":"CLIENT_INVOICE","id":"inv-5"}]},` +
			`{"id":"r3","partnerType":"CLIENT","client":{"id":"cli-9"},"amount":30,"rows":[{"amount":10,"type":"CLIENT_INVOICE","id":"inv-1"},{"amount":20,"type":"CLIENT_INVOICE","id":"inv-6"}]}` +
			`],"hasMoreEntries":false}`
	})
	got, err := p.ListInvoiceReceipts(saCtx, ListInvoiceReceiptsInput{CustomerID: "cli-9", InvoiceID: "inv-1", Since: saDay(2025, 8, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "r1" || got[1].ID != "r3" {
		t.Fatalf("receipts on inv-1 = %+v", got)
	}
	if !got[0].TouchesOnly("inv-1") || got[1].TouchesOnly("inv-1") || !got[1].AmountOn("inv-1").Equal(d("10")) {
		t.Errorf("r1 must be deletable for inv-1, r3 (spans two invoices) must not")
	}
	assertSAWire(t, f, saTestKey(t),
		`GET purchasesales/payments:get?apikey=KEY&dateFrom=01.08.2025&fetchComments=true&fetchRows=true&pageNumber=1&partnerId=cli-9&partnerType=CLIENT`,
	)

	foreign, _ := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"payments":[{"id":"r1","partnerType":"CLIENT","client":{"id":"cli-1"},"amount":10}],"hasMoreEntries":false}`
	})
	if _, err := foreign.ListInvoiceReceipts(saCtx, ListInvoiceReceiptsInput{CustomerID: "cli-9"}); !errors.Is(err, ErrPartnerFilterIgnored) {
		t.Fatalf("want ErrPartnerFilterIgnored, got %v", err)
	}
	unreadable, _ := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"payments":[{"id":"r1","amount":{"x":1}}],"hasMoreEntries":false}`
	})
	if _, err := unreadable.ListInvoiceReceipts(saCtx, ListInvoiceReceiptsInput{CustomerID: "cli-9"}); !errors.Is(err, ErrUnreadablePayment) || errors.Is(err, ErrPartnerFilterIgnored) {
		t.Fatalf("want ErrUnreadablePayment, got %v", err)
	}
	paged, pf := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"payments":[{"id":"r1","partnerType":"CLIENT","client":{"id":"cli-9"},"amount":10}],"hasMoreEntries":true}`
	})
	if _, err := paged.ListInvoiceReceipts(saCtx, ListInvoiceReceiptsInput{CustomerID: "cli-9"}); !errors.Is(err, ErrTooManyPages) {
		t.Fatalf("want ErrTooManyPages, got %v", err)
	}
	if n := len(pf.wire()); n != 5 {
		t.Fatalf("the default cap is 5 pages, sent %d", n)
	}
	if _, err := p.ListInvoiceReceipts(saCtx, ListInvoiceReceiptsInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("a missing customer must be refused, got %v", err)
	}
}

func TestSAListCashAccounts(t *testing.T) {
	p, _ := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"cashAccounts":[{"name":"Kassa","account":"1000","currency":"EUR"}]}`
	})
	got, err := p.ListCashAccounts(saCtx)
	if err != nil || len(got) != 1 || got[0].ID != "Kassa" || got[0].Name != "Kassa" || got[0].AccountCode != "1000" || got[0].CurrencyCode != "EUR" {
		t.Fatalf("ListCashAccounts = %+v, %v", got, err)
	}
	denied, _ := newSAFake(t, func(int, saWire) (int, string) { return 403, `{"error":"no rights"}` })
	if _, err := denied.ListCashAccounts(saCtx); !IsAuthFailed(err) {
		t.Fatalf("a missing privilege must surface as auth-failed, got %v", err)
	}
}

func TestSAListNettingBanks(t *testing.T) {
	p, f := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"bankAccounts":[{"name":"Swedbank","forNetting":false},{"name":"Tasaarveldus","forNetting":true}]}`
	})
	got, err := p.ListNettingBanks(saCtx)
	if err != nil || len(got) != 1 || got[0] != "Tasaarveldus" {
		t.Fatalf("ListNettingBanks = %v, %v", got, err)
	}
	if w := f.wire(); len(w) != 1 || w[0].Path != "settings/bankaccounts:get" {
		t.Fatalf("wire = %v", w)
	}
	denied, _ := newSAFake(t, func(int, saWire) (int, string) { return 403, `{"error":"no rights"}` })
	if _, err := denied.ListNettingBanks(saCtx); !IsAuthFailed(err) {
		t.Fatalf("a missing privilege must surface as auth-failed, got %v", err)
	}
}

func TestReceiptTouchesOnly(t *testing.T) {
	row := func(typ, id, amt string) ReceiptRow { return ReceiptRow{Type: typ, DocID: id, Amount: d(amt)} }
	cases := []struct {
		name string
		r    Receipt
		want bool
	}{
		{"plain", Receipt{Amount: d("10"), Rows: []ReceiptRow{row("CLIENT_INVOICE", "inv-1", "10")}}, true},
		{"other invoice", Receipt{Amount: d("10"), Rows: []ReceiptRow{row("CLIENT_INVOICE", "inv-2", "10")}}, false},
		{"two rows", Receipt{Amount: d("20"), Rows: []ReceiptRow{row("CLIENT_INVOICE", "inv-1", "10"), row("CLIENT_INVOICE", "inv-2", "10")}}, false},
		{"parks remainder", Receipt{Amount: d("30"), AdvanceAmount: d("20"), Rows: []ReceiptRow{row("CLIENT_INVOICE", "inv-1", "10")}}, false},
		{"amount beyond row", Receipt{Amount: d("30"), Rows: []ReceiptRow{row("CLIENT_INVOICE", "inv-1", "10")}}, false},
		{"advance draw", Receipt{Amount: d("0"), Rows: []ReceiptRow{row("PREPAYMENT_PAYMENT", "inv-1", "10")}}, false},
		{"unreadable", Receipt{Amount: d("10"), DecodeError: "x", Rows: []ReceiptRow{row("CLIENT_INVOICE", "inv-1", "10")}}, false},
		{"no rows", Receipt{Amount: d("10")}, false},
		// Extras summing to zero leave the amount equal to the row; only the flag
		// stops the delete from taking the accountant's postings with it.
		{"extras", Receipt{Amount: d("10"), HasExtras: true, Rows: []ReceiptRow{row("CLIENT_INVOICE", "inv-1", "10")}}, false},
	}
	for _, c := range cases {
		if got := c.r.TouchesOnly("inv-1"); got != c.want {
			t.Errorf("%s: TouchesOnly = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestOptionalReceiptInterfacesSmartAccountsOnly(t *testing.T) {
	check := func(name string, p any, want bool) {
		t.Helper()
		_, a := p.(ReceiptCreator)
		_, b := p.(InvoiceReceiptLister)
		_, c := p.(ReceiptGetter)
		_, e := p.(PaymentChangeLister)
		_, g := p.(CashAccountLister)
		_, h := p.(BudgetReporter)
		_, n := p.(NettingBankLister)
		_, ra := p.(PrepaymentReceiptApplier)
		for i, ok := range []bool{a, b, c, e, g, h, n, ra} {
			if ok != want {
				t.Errorf("%s: interface #%d implemented=%v, want %v", name, i, ok, want)
			}
		}
	}
	check("smartaccounts", (*smartProvider)(nil), true)
	check("merit", (*meritProvider)(nil), false)
	check("excellentbooks", (*excellentProvider)(nil), false)
	check("simplbooks", (*simplbooksProvider)(nil), false)
	check("directo", (*directoProvider)(nil), false)

	// A provider without them is refused, never served by the old method.
	c, err := NewClient(Config{Provider: "merit", APIID: "x", APIKey: "y"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Payments.SupportsReceiptCreate() || c.Payments.SupportsReceiptRead() || c.Sync.SupportsPaymentChanges() || c.Taxes.SupportsCashAccounts() || c.Taxes.SupportsNettingBanks() || c.Prepayments.SupportsApplyReceipt() {
		t.Error("merit must report no receipt capabilities")
	}
	var pe *ProviderError
	if _, err := c.Payments.CreateReceipt(saCtx, CreatePaymentInput{}); !errors.As(err, &pe) || !errors.Is(err, ErrUnsupportedProvider) || pe.Provider != "merit" {
		t.Errorf("CreateReceipt on merit = %v", err)
	}
	if _, err := c.Payments.ListInvoiceReceipts(saCtx, ListInvoiceReceiptsInput{}); !errors.Is(err, ErrUnsupportedProvider) {
		t.Errorf("ListInvoiceReceipts on merit = %v", err)
	}
	if _, err := c.Payments.GetReceipt(saCtx, "x"); !errors.Is(err, ErrUnsupportedProvider) {
		t.Errorf("GetReceipt on merit = %v", err)
	}
	if _, err := c.Sync.ListPaymentChanges(saCtx, PaymentChangesInput{}); !errors.Is(err, ErrUnsupportedProvider) {
		t.Errorf("ListPaymentChanges on merit = %v", err)
	}
	if _, err := c.Taxes.ListCashAccounts(saCtx); !errors.Is(err, ErrUnsupportedProvider) {
		t.Errorf("ListCashAccounts on merit = %v", err)
	}
	if _, known := c.BudgetUsed(); known {
		t.Error("merit has no budget meter")
	}

	sa, err := NewClient(Config{Provider: "smartaccounts", APIID: "k-" + t.Name(), APIKey: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if !sa.Payments.SupportsReceiptCreate() || !sa.Payments.SupportsReceiptRead() || !sa.Sync.SupportsPaymentChanges() || !sa.Taxes.SupportsCashAccounts() {
		t.Error("smartaccounts must report every receipt capability")
	}
	if _, known := sa.BudgetUsed(); !known {
		t.Error("smartaccounts meters its budget")
	}
}

// SmartAccounts writes createdBy on every payment: "api" for one made through
// the API and the user's email for one a person entered — a statement import
// included (live, 07.10.2026). An odd value must not fail the row; absent reads
// as unknown, never as the API.
func TestSAReceiptCreatedBy(t *testing.T) {
	p, _ := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"payments":[` +
			`{"id":"ours","date":"08.10.2026","partnerType":"CLIENT","amount":5,"createdBy":"api","document":"KLUBIO-PMT-1"},` +
			`{"id":"person","date":"08.10.2026","partnerType":"CLIENT","amount":15,"createdBy":"accountant@example.ee"},` +
			`{"id":"unsaid","date":"08.10.2026","partnerType":"CLIENT","amount":15},` +
			`{"id":"odd","date":"08.10.2026","partnerType":"CLIENT","amount":15,"createdBy":{"x":1}}` +
			`],"hasMoreEntries":false}`
	})
	ch, err := p.ListPaymentChanges(saCtx, PaymentChangesInput{Since: saDay(2026, 10, 1)})
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Receipt{}
	for _, r := range ch.Receipts {
		by[r.ID] = r
	}
	if r := by["ours"]; r.CreatedBy != "api" || !r.CreatedThroughAPI() {
		t.Errorf("api receipt = %q api=%v", r.CreatedBy, r.CreatedThroughAPI())
	}
	if r := by["person"]; r.CreatedBy != "accountant@example.ee" || r.CreatedThroughAPI() {
		t.Errorf("person receipt = %q api=%v", r.CreatedBy, r.CreatedThroughAPI())
	}
	if r := by["unsaid"]; r.CreatedBy != "" || r.CreatedThroughAPI() {
		t.Errorf("absent createdBy = %q api=%v", r.CreatedBy, r.CreatedThroughAPI())
	}
	if r := by["odd"]; r.DecodeError != "" || r.CreatedThroughAPI() || !r.Amount.Equal(d("15")) {
		t.Errorf("an odd createdBy must neither fail the row nor read as the API: %+v", r)
	}
}
