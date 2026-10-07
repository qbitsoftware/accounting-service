package accounting

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/qbitsoftware/accounting-service/smartaccounts"
)

// The goldens in this file were recorded against v0.14.5, BEFORE any receipt
// work, and pin what every existing SmartAccounts caller sends and decodes.
// Receipt-level behaviour (v0.15.0) is opt-in and must leave every one of them
// byte-identical: a club not moved onto it keeps exactly the requests it sent
// before. A failure here is not a test to update — it is an existing club's
// wire changing.

var saCtx = context.Background()

func saDay(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

const saInvoiceByNumber = `{"clientInvoices":[{"id":"inv-1","clientId":"cli-9","client":{"id":"cli-9","name":"Mari Maasikas"},` +
	`"invoiceNumber":"2026-0042","date":"01.09.2026","currency":"EUR","totalAmount":25.5,"outstandingAmount":25.5}],"hasMoreEntries":false}`

func TestSACreatePaymentWireGolden(t *testing.T) {
	p, f := newSAFake(t, func(n int, w saWire) (int, string) {
		if strings.HasPrefix(w.Path, "purchasesales/clientinvoices:get") {
			return 200, saInvoiceByNumber
		}
		return 200, `{"paymentId":"pay-77","number":"12","amount":25.5}`
	})
	err := p.CreatePayment(saCtx, CreatePaymentInput{
		CustomerName: "Mari Maasikas",
		CustomerCode: "cli-9",
		PaymentNo:    "IGNORED-BY-SA",
		InvoiceNo:    "2026-0042",
		PaymentDate:  saDay(2026, 9, 3),
		Amount:       d("25.50"),
		Currency:     "EUR",
		BankID:       "LHV klubio",
		AutoConfirm:  true,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	assertSAWire(t, f, saTestKey(t),
		`GET purchasesales/clientinvoices:get?apikey=KEY&dateFrom=01.01.2000&fetchRows=true&invoiceNumber=2026-0042&pageNumber=1`,
		`POST purchasesales/payments:add?apikey=KEY {"date":"03.09.2026","partnerType":"CLIENT","clientId":"cli-9","accountType":"BANK","accountName":"LHV klubio","currency":"EUR","amount":"25.5","rows":[{"amount":"25.5","type":"CLIENT_INVOICE","id":"inv-1"}]}`,
	)
}

func TestSAPrepaymentParkWireGolden(t *testing.T) {
	p, f := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"paymentId":"adv-1","number":"5","amount":40}`
	})
	pp, err := p.CreatePrepayment(saCtx, CreatePrepaymentInput{
		CustomerCode: "cli-9",
		PrepaymentNo: "KLB-PP-3",
		Amount:       d("40"),
		Currency:     "EUR",
		PaymentDate:  saDay(2026, 9, 4),
		BankID:       "LHV klubio",
		Account:      "21211",
		Comment:      "Ettemaks",
	})
	if err != nil {
		t.Fatalf("CreatePrepayment: %v", err)
	}
	if pp.Number != "adv-1" || pp.DocID != "adv-1" {
		t.Errorf("prepayment handle = %q/%q, want adv-1", pp.Number, pp.DocID)
	}
	assertSAWire(t, f, saTestKey(t),
		`POST purchasesales/payments:add?apikey=KEY {"date":"04.09.2026","document":"KLB-PP-3","partnerType":"CLIENT","clientId":"cli-9","accountType":"BANK","accountName":"LHV klubio","currency":"EUR","amount":"40","prepayment":{"amount":"40","description":"Ettemaks","account":"21211"}}`,
	)
}

func TestSAPrepaymentApplyWireGolden(t *testing.T) {
	p, f := newSAFake(t, func(n int, w saWire) (int, string) {
		if strings.HasPrefix(w.Path, "purchasesales/clientinvoices:get") {
			return 200, saInvoiceByNumber
		}
		return 200, `{"paymentId":"app-1","number":"6","amount":0}`
	})
	err := p.ApplyPrepayment(saCtx, ApplyPrepaymentInput{
		CustomerCode: "cli-9",
		InvoiceNo:    "2026-0042",
		PrepaymentNo: "adv-1",
		Amount:       d("20"),
		Currency:     "EUR",
		PaymentDate:  saDay(2026, 9, 5),
		BankID:       "LHV klubio",
	})
	if err != nil {
		t.Fatalf("ApplyPrepayment: %v", err)
	}
	assertSAWire(t, f, saTestKey(t),
		`GET purchasesales/clientinvoices:get?apikey=KEY&dateFrom=01.01.2000&fetchRows=true&invoiceNumber=2026-0042&pageNumber=1`,
		`POST purchasesales/payments:add?apikey=KEY {"date":"05.09.2026","partnerType":"CLIENT","clientId":"cli-9","accountType":"BANK","accountName":"LHV klubio","currency":"EUR","amount":"0","rows":[{"amount":"20","type":"CLIENT_INVOICE","id":"inv-1"},{"amount":"20","type":"PREPAYMENT_PAYMENT","id":"adv-1"}]}`,
	)
}

func TestSASettleAgainstCreditWireGolden(t *testing.T) {
	p, f := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"paymentId":"net-1","number":"7","amount":60}`
	})
	if _, err := p.client.SettleInvoiceAgainstCredit(saCtx, "cli-9", "inv-2", "cre-2", d("30"), "EUR", "06.09.2026"); err != nil {
		t.Fatalf("SettleInvoiceAgainstCredit: %v", err)
	}
	assertSAWire(t, f, saTestKey(t),
		`POST purchasesales/payments:add?apikey=KEY {"date":"06.09.2026","partnerType":"CLIENT","clientId":"cli-9","accountType":"BANK","accountName":"Tasaarveldus","currency":"EUR","amount":"60","comment":"auto-settle credit against original","rows":[{"amount":"30","type":"CLIENT_INVOICE","id":"inv-2"},{"amount":"30","type":"CLIENT_INVOICE","id":"cre-2"}]}`,
	)
}

func TestSADeletePaymentWireGolden(t *testing.T) {
	p, f := newSAFake(t, nil)
	if err := p.DeletePayment(saCtx, "pay-77"); err != nil {
		t.Fatalf("DeletePayment: %v", err)
	}
	assertSAWire(t, f, saTestKey(t),
		`POST purchasesales/payments:delete?apikey=KEY&id=pay-77 {}`,
	)
}

func TestSAGetInvoiceLeavesPaymentsEmpty(t *testing.T) {
	// deleteMeritPayments deletes every entry of Invoices.Get(...).Payments and is
	// not provider-gated, so a SmartAccounts invoice that started reporting its
	// receipts here would have them all deleted on the next re-issue.
	p, f := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"clientInvoices":[{"id":"inv-1","clientId":"cli-9","type":"","invoiceNumber":"2026-0042","date":"01.09.2026",` +
			`"currency":"EUR","totalAmount":25.5,"outstandingAmount":5.5}],"hasMoreEntries":false}`
	})
	inv, err := p.GetInvoice(saCtx, "inv-1")
	if err != nil {
		t.Fatalf("GetInvoice: %v", err)
	}
	if inv.Payments != nil {
		t.Fatalf("GetInvoice must leave Payments nil for SmartAccounts, got %+v", inv.Payments)
	}
	if !inv.PaidAmount.Equal(d("20")) {
		t.Errorf("paid = %s, want 20", inv.PaidAmount)
	}
	assertSAWire(t, f, saTestKey(t),
		`GET purchasesales/clientinvoices:get?apikey=KEY&fetchRows=true&id=inv-1&pageNumber=1`,
	)
}

func TestSAListPaymentsSinceQueryGolden(t *testing.T) {
	p, f := newSAFake(t, func(int, saWire) (int, string) { return 200, `{"payments":[],"hasMoreEntries":false}` })
	if _, err := p.ListPaymentsSince(saCtx, saDay(2026, 7, 9), saDay(2027, 1, 5)); err != nil {
		t.Fatalf("ListPaymentsSince: %v", err)
	}
	if _, err := p.ListPayments(saCtx, ListPaymentsInput{PeriodStart: saDay(2026, 7, 1), PeriodEnd: saDay(2026, 7, 31)}); err != nil {
		t.Fatalf("ListPayments: %v", err)
	}
	assertSAWire(t, f, saTestKey(t),
		`GET purchasesales/payments:get?apikey=KEY&dateFrom=09.07.2026&dateTo=05.01.2027&dateType=modifydate&fetchRows=true&pageNumber=1`,
		`GET purchasesales/payments:get?apikey=KEY&dateFrom=01.07.2026&dateTo=31.07.2026&fetchRows=true&pageNumber=1`,
	)
}

// saFeedPage1 / saFeedPage2 are one modifydate feed read covering every payment
// shape the pull meets: an advance park, an advance applied to an invoice, the
// credit-note netting klubio posts, a cash receipt and a vendor payment.
//
// Shapes checked against the live SmartAccounts feed of 07.10.2026 (the
// sandbox company's payments:get?dateType=modifydate&fetchRows=true; ids
// shortened, everything else as SmartAccounts sent it):
//   - `deleted` is page 1's array of objects {accountType, date, id, number}, and
//     the key is OMITTED, not sent empty, when nothing was deleted (page 2 here);
//   - every payment carries createdBy ("api" for one made through the API, the
//     user's email for one a person entered), dateCreated and exchangeRate;
//   - number and orderNumber are bare JSON numbers;
//   - an advance park (adv-1) has no rows and a top-level prepayment
//     {account, amount, description};
//   - an application (app-1) has amount 0 and its PREPAYMENT_PAYMENT row is
//     NEGATIVE (the draw), naming the advance's receipt id — the rows sum to the
//     payment's amount;
//   - a cash receipt (cash-1) is accountType CASH in the desk "Kassa / Cash".
//
// Still synthetic: the netting receipt (net-1; sandbox checklist item 8 is
// unrecorded) and the vendor payment (ven-1; the sandbox company has none).
const saFeedPage1 = `{"deleted":[` + saFeedDeleted + `],` + saFeedPage1Body

// saFeedPage1Legacy is page 1 on a window with nothing deleted: SmartAccounts
// then omits the key. The v0.14.5 read decodes `deleted` as strings and fails
// the whole page on the live object shape (TestSAListPaymentsSinceLiveDeletedShape),
// so its decode golden is pinned on this one.
const saFeedPage1Legacy = `{` + saFeedPage1Body

const saFeedDeleted = `{"accountType":"CASH","date":"08.10.2026","id":"gone-1","number":4},` +
	`{"accountType":"BANK","date":"07.10.2026","id":"gone-2","number":1}`

const saFeedPage1Body = `"hasMoreEntries":true,"payments":[` +
	`{"accountName":"LHV klubio","accountType":"BANK","amount":50,"client":{"id":"cli-9","name":"Mari Maasikas"},"clientId":"cli-9",` +
	`"createdBy":"api","currency":"EUR","date":"01.09.2026","dateCreated":"01.09.2026 22:48:47","document":"KLB-PP-3","exchangeRate":1,` +
	`"id":"adv-1","number":5,"partnerType":"CLIENT","prepayment":{"account":"21211","amount":50,"description":"KLB-PP-3 Overpayment on invoice RSA000003"}},` +
	`{"accountName":"LHV klubio","accountType":"BANK","amount":0,"client":{"id":"cli-9","name":"Mari Maasikas"},"clientId":"cli-9",` +
	`"createdBy":"api","currency":"EUR","date":"02.09.2026","dateCreated":"02.09.2026 23:21:46","document":"KLUBIO-APPLY-2002376","exchangeRate":1,` +
	`"id":"app-1","number":6,"partnerType":"CLIENT","rows":[` +
	`{"amount":-20,"description":"Ettemakse (02.09.2026)","id":"adv-1","type":"PREPAYMENT_PAYMENT"},` +
	`{"amount":20,"description":"RSA000008 (02.09.2026)","id":"inv-1","type":"CLIENT_INVOICE"}]},` +
	`{"id":"net-1","date":"03.09.2026","number":"7","partnerType":"CLIENT","clientId":"cli-8",` +
	`"client":{"id":"cli-8","name":"Jaan Tamm"},"accountType":"BANK","accountName":"Tasaarveldus","currency":"EUR","amount":60,` +
	`"rows":[{"amount":30,"type":"CLIENT_INVOICE","id":"inv-2"},{"amount":30,"type":"CLIENT_INVOICE","id":"cre-2"}]}` +
	`]}`

const saFeedPage2 = `{"hasMoreEntries":false,"payments":[` +
	`{"accountName":"Kassa","accountType":"CASH","amount":15,"client":{"id":"cli-7","name":"Kati Kask"},"clientId":"cli-7",` +
	`"createdBy":"api","currency":"EUR","date":"04.09.2026","dateCreated":"04.09.2026 00:34:11","document":"KLUBIO-PMT-2002386","exchangeRate":1,` +
	`"id":"cash-1","number":17,"orderNumber":3,"partnerType":"CLIENT",` +
	`"rows":[{"amount":15,"description":"RSA000015 (04.09.2026)","id":"inv-3","type":"CLIENT_INVOICE"}]},` +
	`{"id":"ven-1","date":"05.09.2026","number":"8","partnerType":"VENDOR","vendorId":"ven-9",` +
	`"vendor":{"id":"ven-9","name":"Saalirent OÜ"},"accountType":"BANK","accountName":"LHV klubio","currency":"EUR","amount":300,` +
	`"rows":[{"amount":300,"type":"VENDOR_INVOICE","id":"vi-1"}]}` +
	`]}`

func renderSAPayments(ps []Payment) string {
	var b strings.Builder
	for _, p := range ps {
		fmt.Fprintf(&b, "%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|", p.ID, p.DocumentNo, p.DocumentDate.Format("2006-01-02"),
			p.Amount, p.Currency, p.Direction, p.CounterPartID, p.CounterPartName, p.ExternalPayMode, p.ExternalBankName)
		for _, l := range p.InvoiceLinks {
			fmt.Fprintf(&b, "[%s %q %s fp=%s]", l.InvoiceID, l.InvoiceNo, l.Amount, l.FromPrepayment)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func TestSAListPaymentsSinceDecodeGolden(t *testing.T) {
	p, _ := newSAFake(t, func(n int, w saWire) (int, string) {
		if strings.Contains(w.Query, "pageNumber=2") {
			return 200, saFeedPage2
		}
		return 200, saFeedPage1Legacy
	})
	got, err := p.ListPaymentsSince(saCtx, saDay(2026, 7, 9), saDay(2027, 1, 5))
	if err != nil {
		t.Fatalf("ListPaymentsSince: %v", err)
	}
	want := "adv-1|5|2026-09-01|50|EUR|customer|cli-9|Mari Maasikas|LHV klubio|LHV klubio|\n" +
		"app-1|6|2026-09-02|0|EUR|customer|cli-9|Mari Maasikas|LHV klubio|LHV klubio|[inv-1 \"\" 20 fp=0]\n" +
		"net-1|7|2026-09-03|60|EUR|customer|cli-8|Jaan Tamm|Tasaarveldus|Tasaarveldus|[inv-2 \"\" 30 fp=0][cre-2 \"\" 30 fp=0]\n" +
		"cash-1|17|2026-09-04|15|EUR|customer|cli-7|Kati Kask|Kassa|Kassa|[inv-3 \"\" 15 fp=0]\n" +
		"ven-1|8|2026-09-05|300|EUR|vendor|ven-9|Saalirent OÜ|LHV klubio|LHV klubio|[vi-1 \"\" 300 fp=0]\n"
	if r := renderSAPayments(got); r != want {
		t.Fatalf("decode changed\n got:\n%s\n want:\n%s", r, want)
	}
}

// TestSAListPaymentsSinceLiveDeletedShape pins a gap in the v0.14.5 read, not
// a wish: it decodes `deleted` as a list of strings, and SmartAccounts sends
// objects ({accountType, date, id, number}; live 07.10.2026) whenever anything
// in the window was deleted — so that read fails outright for such a window.
// It is what every unlisted club's pull uses and is deliberately left as it
// is; the receipt-level read (ListPaymentChanges) reads the shape. Changing
// this answer changes unlisted clubs' pulls and must be done on purpose.
func TestSAListPaymentsSinceLiveDeletedShape(t *testing.T) {
	p, _ := newSAFake(t, func(n int, w saWire) (int, string) {
		if strings.Contains(w.Query, "pageNumber=2") {
			return 200, saFeedPage2
		}
		return 200, saFeedPage1
	})
	if _, err := p.ListPaymentsSince(saCtx, saDay(2026, 7, 9), saDay(2027, 1, 5)); err == nil ||
		!strings.Contains(err.Error(), "unmarshal page meta") {
		t.Fatalf("ListPaymentsSince on a live deleted list = %v, want the page-meta decode failure it has always had", err)
	}
}

func TestMapSAPaymentGolden(t *testing.T) {
	item := smartaccounts.PaymentItem{
		ID:          "p1",
		Date:        "10.03.2025",
		Number:      "PAY-1",
		Document:    "KLUBIO-PMT-9",
		PartnerType: smartaccounts.PartnerClient,
		ClientID:    "9",
		Client:      &smartaccounts.PartnerRef{ID: "9", Name: "Acme OÜ", RegCode: "123", VatNumber: "EE1"},
		AccountType: smartaccounts.AccountCash,
		AccountName: "Kassa",
		Currency:    "EUR",
		Amount:      d("120"),
		Rows: []smartaccounts.PaymentRow{
			{Type: smartaccounts.RowClientInvoice, ID: "55", Amount: d("100"), Description: "x"},
			{Type: smartaccounts.RowPrepayment, ID: "adv-1", Amount: d("20")},
			{Type: smartaccounts.RowVendorInvoice, ID: "v1", Amount: d("1")},
		},
	}
	want := "p1|PAY-1|2025-03-10|120|EUR|customer|9|Acme OÜ|Kassa|Kassa|[55 \"\" 100 fp=0][v1 \"\" 1 fp=0]\n"
	if r := renderSAPayments([]Payment{mapSAPayment(item)}); r != want {
		t.Fatalf("mapSAPayment changed\n got:  %s want: %s", r, want)
	}
	// A vendor-only payment falls back to the vendor as the counterpart.
	ven := mapSAPayment(smartaccounts.PaymentItem{ID: "v", PartnerType: smartaccounts.PartnerVendor,
		Vendor: &smartaccounts.PartnerRef{ID: "ven-9", Name: "Saal"}})
	if ven.CounterPartID != "ven-9" || ven.Direction != PaymentDirectionVendor || ven.InvoiceLinks == nil || len(ven.InvoiceLinks) != 0 {
		t.Fatalf("vendor mapping changed: %+v", ven)
	}
}

// A cash overpayment's advance is parked in the cash desk it was paid into,
// not a bank (accountType=CASH); only the account type differs.
func TestSAPrepaymentCreateCashDeskWire(t *testing.T) {
	p, f := newSAFake(t, func(int, saWire) (int, string) {
		return 200, `{"paymentId":"adv-2","number":"7","amount":10}`
	})
	if _, err := p.CreatePrepayment(saCtx, CreatePrepaymentInput{
		CustomerCode: "cli-9",
		PrepaymentNo: "KLB-PP-4",
		Amount:       d("10"),
		Currency:     "EUR",
		PaymentDate:  saDay(2026, 10, 7),
		BankID:       "Kassa / Cash",
		Account:      "21211",
		Comment:      "Ettemaks",
		AccountKind:  PaymentAccountCash,
	}); err != nil {
		t.Fatalf("CreatePrepayment: %v", err)
	}
	assertSAWire(t, f, saTestKey(t),
		`POST purchasesales/payments:add?apikey=KEY {"date":"07.10.2026","document":"KLB-PP-4","partnerType":"CLIENT","clientId":"cli-9","accountType":"CASH","accountName":"Kassa / Cash","currency":"EUR","amount":"10","prepayment":{"amount":"10","description":"Ettemaks","account":"21211"}}`,
	)
}
