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
// credit-note netting klubio posts, a cash receipt (orderNumber present, number
// as a bare JSON number) and a vendor payment.
//
// TODO(before tagging v0.15.0): these pages are SYNTHETIC — written from the
// API spec, not recorded. The `deleted` element type, the orderNumber shape, the
// advance park / apply / netting shapes and the cash payload are all unproven.
// Replace or extend them with raw payloads recorded from the SmartAccounts
// sandbox (design §2.5; checklist Gate A items 3, 5, 8, 9 — S-a/b/c/f) and
// re-run the decode goldens. Until then the branch stays untagged.
const saFeedPage1 = `{"payments":[` +
	`{"id":"adv-1","date":"01.09.2026","number":"5","document":"KLB-PP-3","partnerType":"CLIENT","clientId":"cli-9",` +
	`"client":{"id":"cli-9","name":"Mari Maasikas"},"accountType":"BANK","accountName":"LHV klubio","currency":"EUR","amount":50,` +
	`"prepayment":{"amount":50,"account":"21211"}},` +
	`{"id":"app-1","date":"02.09.2026","number":"6","partnerType":"CLIENT","clientId":"cli-9",` +
	`"client":{"id":"cli-9","name":"Mari Maasikas"},"accountType":"BANK","accountName":"LHV klubio","currency":"EUR","amount":0,` +
	`"rows":[{"amount":20,"type":"CLIENT_INVOICE","id":"inv-1"},{"amount":20,"type":"PREPAYMENT_PAYMENT","id":"adv-1"}]},` +
	`{"id":"net-1","date":"03.09.2026","number":"7","partnerType":"CLIENT","clientId":"cli-8",` +
	`"client":{"id":"cli-8","name":"Jaan Tamm"},"accountType":"BANK","accountName":"Tasaarveldus","currency":"EUR","amount":60,` +
	`"rows":[{"amount":30,"type":"CLIENT_INVOICE","id":"inv-2"},{"amount":30,"type":"CLIENT_INVOICE","id":"cre-2"}]}` +
	`],"deleted":["gone-1","gone-2"],"hasMoreEntries":true}`

const saFeedPage2 = `{"payments":[` +
	`{"id":"cash-1","date":"04.09.2026","number":17,"orderNumber":3,"partnerType":"CLIENT","clientId":"cli-7",` +
	`"client":{"id":"cli-7","name":"Kati Kask"},"accountType":"CASH","accountName":"Kassa","currency":"EUR","amount":15,` +
	`"rows":[{"amount":15,"type":"CLIENT_INVOICE","id":"inv-3"}]},` +
	`{"id":"ven-1","date":"05.09.2026","number":"8","partnerType":"VENDOR","vendorId":"ven-9",` +
	`"vendor":{"id":"ven-9","name":"Saalirent OÜ"},"accountType":"BANK","accountName":"LHV klubio","currency":"EUR","amount":300,` +
	`"rows":[{"amount":300,"type":"VENDOR_INVOICE","id":"vi-1"}]}` +
	`],"hasMoreEntries":false}`

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
		return 200, saFeedPage1
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
