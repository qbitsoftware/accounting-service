package accounting

import (
	"context"
	"strings"
	"testing"
	"time"
)

// The goldens in this file were recorded on eb-receipts BEFORE any receipt-
// level work for Excellent Books, and pin what every existing EB caller sends
// to the receipt register (IPVc) and what it decodes from it. The receipt-level
// capabilities (excellentbooks_receipts.go) are opt-in and must leave every one
// of them byte-identical: a club that is not moved onto them keeps exactly the
// requests it sent before. A failure here is not a test to update — it is an
// existing club's wire changing.

var ebCtx = context.Background()

func TestEBCreatePaymentWireGolden(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) { return 200, ebCreated("300001") })
	err := p.CreatePayment(ebCtx, CreatePaymentInput{
		CustomerName: "Mari Maasikas",
		CustomerCode: "K7224",
		PaymentNo:    "IGNORED-BY-EB",
		InvoiceNo:    "260123",
		PaymentDate:  ebDay(2026, 9, 3),
		Amount:       d("25.50"),
		Currency:     "EUR",
		BankID:       "P2",
		AutoConfirm:  true,
		// The receipt fields belong to CreateReceipt; CreatePayment must never
		// start sending them.
		AccountKind:     PaymentAccountCash,
		Document:        "KLUBIO-PMT-1",
		Comment:         "ignored",
		ExpectInvoiceID: "260123",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	assertEBWire(t, f,
		`POST IPVc set_field.OKFlag=1&set_field.PayCurCode=EUR&set_field.PayMode=P2&set_field.TransDate=2026-09-03&set_row_field.0.CustCode=K7224&set_row_field.0.InvoiceNr=260123&set_row_field.0.PayDate=2026-09-03&set_row_field.0.RecVal=25.5&set_row_field.0.stp=1`,
	)
}

func TestEBCreatePaymentMinimalWireGolden(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) { return 200, ebCreated("300002") })
	// No currency, no customer code (the name is the fallback), zero amount
	// (RecVal left to EB, which takes the invoice's open amount).
	err := p.CreatePayment(ebCtx, CreatePaymentInput{
		CustomerName: "Mari Maasikas",
		InvoiceNo:    "260124",
		PaymentDate:  ebDay(2026, 9, 4),
		BankID:       "P1",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	assertEBWire(t, f,
		`POST IPVc set_field.OKFlag=1&set_field.PayMode=P1&set_field.TransDate=2026-09-04&set_row_field.0.CustCode=Mari%20Maasikas&set_row_field.0.InvoiceNr=260124&set_row_field.0.PayDate=2026-09-04&set_row_field.0.stp=1`,
	)
}

func TestEBCreatePaymentWithoutPayModeSendsNothing(t *testing.T) {
	p, f := newEBFake(t, nil)
	if err := p.CreatePayment(ebCtx, CreatePaymentInput{CustomerCode: "K1", InvoiceNo: "1", Amount: d("1")}); err == nil {
		t.Fatal("CreatePayment without a PayMode must fail")
	}
	assertEBWire(t, f)
}

func TestEBListPaymentsWireGolden(t *testing.T) {
	p, f := newEBFake(t, nil)
	if _, err := p.ListPayments(ebCtx, ListPaymentsInput{PeriodStart: ebDay(2026, 7, 1), PeriodEnd: ebDay(2026, 7, 31)}); err != nil {
		t.Fatalf("ListPayments: %v", err)
	}
	if _, err := p.ListPayments(ebCtx, ListPaymentsInput{PeriodStart: ebDay(2026, 7, 1)}); err != nil {
		t.Fatalf("ListPayments open-ended: %v", err)
	}
	if _, err := p.ListPayments(ebCtx, ListPaymentsInput{}); err != nil {
		t.Fatalf("ListPayments unbounded: %v", err)
	}
	if _, err := p.ListPaymentsSince(ebCtx, ebDay(2026, 4, 9), ebDay(2026, 10, 14)); err != nil {
		t.Fatalf("ListPaymentsSince: %v", err)
	}
	if _, err := p.ListPaymentsSince(ebCtx, ebDay(2026, 4, 9), time.Time{}); err != nil {
		t.Fatalf("ListPaymentsSince open-ended: %v", err)
	}
	assertEBWire(t, f,
		`GET IPVc?limit=5000&range=2026-07-01%3A2026-07-31&sort=TransDate`,
		`GET IPVc?limit=5000&range=2026-07-01%3A&sort=TransDate`,
		`GET IPVc?limit=5000`,
		`GET IPVc?limit=5000&range=2026-04-09%3A2026-10-14&sort=TransDate`,
		`GET IPVc?limit=5000&range=2026-04-09%3A&sort=TransDate`,
	)
}

// ebReceiptFixture is one IPVc list covering every receipt shape a pull meets:
// a cash payment, an advance drawn onto an invoice, paid money freed into an
// advance, an advance parked on its own, one transfer split over two invoices,
// an overpayment parked beside the invoice payment, an unconfirmed (draft)
// receipt and a row of another type (stp 2). Field names and the data
// envelope are those of the live receipts cmd/eb-prepay-probe read; the values
// are synthetic.
const ebReceiptFixture = `{"data":{"@register":"IPVc","@sequence":"5120","@systemversion":"8.5",` +
	`"IPVc":[` +
	`{"@sequence":"5101","@url":"/api/1/IPVc/1001","SerNr":"1001","TransDate":"2026-09-01","PayMode":"P2","Comment":"","OKFlag":"1","PayCurCode":"EUR",` +
	`"rows":[{"@rownumber":"0","stp":"1","InvoiceNr":"260100","CustCode":"K1","CustName":"Mari Maasikas","RecVal":"45.00","PayDate":"2026-09-01","BankVal":"45.00","Comment":"arve 260100"}]},` +
	`{"@sequence":"5102","@url":"/api/1/IPVc/1002","SerNr":"1002","TransDate":"2026-09-02","PayMode":"P2","Comment":"KLUBIO-APPLY-77","OKFlag":"1","PayCurCode":"EUR",` +
	`"rows":[{"@rownumber":"0","stp":"1","InvoiceNr":"260101","CustCode":"K1","RecVal":"75.00","PayDate":"2026-09-02"},` +
	`{"@rownumber":"1","stp":"1","CUPNr":"90000006","CustCode":"K1","RecVal":"-75.00","PayDate":"2026-09-02"}]},` +
	`{"@sequence":"5103","@url":"/api/1/IPVc/1003","SerNr":"1003","TransDate":"2026-09-03","PayMode":"P2","OKFlag":"1","PayCurCode":"EUR",` +
	`"rows":[{"@rownumber":"0","stp":"1","InvoiceNr":"260102","CustCode":"K2","RecVal":"-120.00","PayDate":"2026-09-03"},` +
	`{"@rownumber":"1","stp":"1","CUPNr":"90000000","CustCode":"K2","RecVal":"120.00","PayDate":"2026-09-03"}]},` +
	`{"@sequence":"5104","@url":"/api/1/IPVc/1004","SerNr":"1004","TransDate":"2026-09-04","PayMode":"P1","OKFlag":"1","PayCurCode":"EUR",` +
	`"rows":[{"@rownumber":"0","stp":"1","CUPNr":"KLB-PP-3","CustCode":"K3","RecVal":"40.00","PayDate":"2026-09-04","Comment":"Ettemaks"}]},` +
	`{"@sequence":"5105","@url":"/api/1/IPVc/1005","SerNr":"1005","TransDate":"2026-09-05","PayMode":"P2","OKFlag":"1","PayCurCode":"EUR",` +
	`"rows":[{"@rownumber":"0","stp":"1","InvoiceNr":"260103","CustCode":"K4","RecVal":"30.00","PayDate":"2026-09-05"},` +
	`{"@rownumber":"1","stp":"1","InvoiceNr":"260104","CustCode":"K4","RecVal":"50.00","PayDate":"2026-09-05"}]},` +
	`{"@sequence":"5106","@url":"/api/1/IPVc/1006","SerNr":"1006","TransDate":"2026-09-06","PayMode":"P2","OKFlag":"1","PayCurCode":"EUR",` +
	`"rows":[{"@rownumber":"0","stp":"1","InvoiceNr":"260105","CustCode":"K5","RecVal":"90.00","PayDate":"2026-09-06"},` +
	`{"@rownumber":"1","stp":"1","CUPNr":"2704","CustCode":"K5","RecVal":"30.00","PayDate":"2026-09-06"}]},` +
	`{"@sequence":"5107","@url":"/api/1/IPVc/1007","SerNr":"1007","TransDate":"2026-09-07","PayMode":"P2","OKFlag":"0","PayCurCode":"EUR",` +
	`"rows":[{"@rownumber":"0","stp":"1","InvoiceNr":"260106","CustCode":"K6","RecVal":"15.00","PayDate":"2026-09-07"}]},` +
	`{"@sequence":"5108","@url":"/api/1/IPVc/1008","SerNr":"1008","TransDate":"2026-09-08","PayMode":"P2","OKFlag":"1","PayCurCode":"EUR",` +
	`"rows":[{"@rownumber":"0","stp":"1","InvoiceNr":"260107","CustCode":"K7","RecVal":"20.00","PayDate":"2026-09-08"},` +
	`{"@rownumber":"1","stp":"2","CustCode":"K7","RecVal":"0.40","PayDate":"2026-09-08","Comment":"pangatasu"}]}` +
	`]}}`

// ebPaymentsFixtureWant is ListPayments' projection of ebReceiptFixture, as
// recorded before the receipt work. ListPaymentChanges' Payments must be this
// projection too.
const ebPaymentsFixtureWant = "1001|1001|2026-09-01|45|EUR|customer|||P2||[260100 \"260100\" 45 fp=0]\n" +
	"1002|1002|2026-09-02|0|EUR|customer|||P2||[260101 \"260101\" 75 fp=75]\n" +
	"1003|1003|2026-09-03|0|EUR|customer|||P2||[260102 \"260102\" -120 fp=-120]\n" +
	"1004|1004|2026-09-04|40|EUR|customer|||P1||\n" +
	"1005|1005|2026-09-05|80|EUR|customer|||P2||[260103 \"260103\" 30 fp=0][260104 \"260104\" 50 fp=0]\n" +
	"1006|1006|2026-09-06|120|EUR|customer|||P2||[260105 \"260105\" 90 fp=0]\n" +
	"1007|1007|2026-09-07|15|EUR|customer|||P2||[260106 \"260106\" 15 fp=0]\n" +
	"1008|1008|2026-09-08|20.4|EUR|customer|||P2||[260107 \"260107\" 20 fp=0]\n"

func TestEBListPaymentsDecodeGolden(t *testing.T) {
	p, _ := newEBFake(t, func(int, ebWire) (int, string) { return 200, ebReceiptFixture })
	got, err := p.ListPaymentsSince(ebCtx, ebDay(2026, 9, 1), ebDay(2026, 9, 30))
	if err != nil {
		t.Fatalf("ListPaymentsSince: %v", err)
	}
	if r := renderSAPayments(got); r != ebPaymentsFixtureWant {
		t.Fatalf("decode changed\n got:\n%s\n want:\n%s", r, ebPaymentsFixtureWant)
	}
}

func TestEBCreatePrepaymentWireGolden(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) { return 200, ebCreated("300003") })
	if _, err := p.CreatePrepayment(ebCtx, CreatePrepaymentInput{
		CustomerCode: "K7224",
		PrepaymentNo: "KLB-PP-3",
		Amount:       d("40"),
		Currency:     "EUR",
		PaymentDate:  ebDay(2026, 9, 4),
		BankID:       "P2",
		Account:      "21211",
		Comment:      "Ettemaks arvelt 2026-0042",
		AccountKind:  PaymentAccountCash,
	}); err != nil {
		t.Fatalf("CreatePrepayment: %v", err)
	}
	if _, err := p.CreatePrepayment(ebCtx, CreatePrepaymentInput{
		CustomerCode: "K7224",
		PrepaymentNo: "KLB-PP-4",
		Amount:       d("12.30"),
		PaymentDate:  ebDay(2026, 9, 5),
		BankID:       "P2",
	}); err != nil {
		t.Fatalf("CreatePrepayment minimal: %v", err)
	}
	assertEBWire(t, f,
		`POST IPVc set_field.OKFlag=1&set_field.PayCurCode=EUR&set_field.PayMode=P2&set_field.TransDate=2026-09-04&set_row_field.0.CUPNr=KLB-PP-3&set_row_field.0.Comment=Ettemaks%20arvelt%202026-0042&set_row_field.0.CustCode=K7224&set_row_field.0.PayDate=2026-09-04&set_row_field.0.RecVal=40&set_row_field.0.stp=1`,
		`POST IPVc set_field.OKFlag=1&set_field.PayMode=P2&set_field.TransDate=2026-09-05&set_row_field.0.CUPNr=KLB-PP-4&set_row_field.0.CustCode=K7224&set_row_field.0.PayDate=2026-09-05&set_row_field.0.RecVal=12.3&set_row_field.0.stp=1`,
	)
}

func TestEBApplyPrepaymentWireGolden(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) { return 200, ebCreated("300004") })
	if err := p.ApplyPrepayment(ebCtx, ApplyPrepaymentInput{
		CustomerCode: "K7224",
		InvoiceNo:    "260123",
		PrepaymentNo: "KLB-PP-3",
		Amount:       d("20"),
		Currency:     "EUR",
		PaymentDate:  ebDay(2026, 9, 5),
		BankID:       "P2",
		// Document belongs to ApplyPrepaymentReceipt; ApplyPrepayment must
		// never start sending it.
		Document: "KLUBIO-APPLY-77",
	}); err != nil {
		t.Fatalf("ApplyPrepayment: %v", err)
	}
	if err := p.ApplyPrepayment(ebCtx, ApplyPrepaymentInput{
		CustomerCode: "K7224",
		InvoiceNo:    "260124",
		PrepaymentNo: "KLB-PP-3",
		Amount:       d("7.5"),
		PaymentDate:  ebDay(2026, 9, 6),
		BankID:       "P2",
	}); err != nil {
		t.Fatalf("ApplyPrepayment minimal: %v", err)
	}
	assertEBWire(t, f,
		`POST IPVc set_field.OKFlag=1&set_field.PayCurCode=EUR&set_field.PayMode=P2&set_field.TransDate=2026-09-05&set_row_field.0.CustCode=K7224&set_row_field.0.InvoiceNr=260123&set_row_field.0.PayDate=2026-09-05&set_row_field.0.RecVal=20&set_row_field.0.stp=1&set_row_field.1.CUPNr=KLB-PP-3&set_row_field.1.CustCode=K7224&set_row_field.1.PayDate=2026-09-05&set_row_field.1.RecVal=-20&set_row_field.1.stp=1`,
		`POST IPVc set_field.OKFlag=1&set_field.PayMode=P2&set_field.TransDate=2026-09-06&set_row_field.0.CustCode=K7224&set_row_field.0.InvoiceNr=260124&set_row_field.0.PayDate=2026-09-06&set_row_field.0.RecVal=7.5&set_row_field.0.stp=1&set_row_field.1.CUPNr=KLB-PP-3&set_row_field.1.CustCode=K7224&set_row_field.1.PayDate=2026-09-06&set_row_field.1.RecVal=-7.5&set_row_field.1.stp=1`,
	)
}

func TestEBUnallocateToPrepaymentWireGolden(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) { return 200, ebCreated("300005") })
	if _, err := p.UnallocateToPrepayment(ebCtx, UnallocateToPrepaymentInput{
		CustomerCode: "K7224",
		InvoiceNo:    "260123",
		PrepaymentNo: "KLB-CN-9",
		Amount:       d("120"),
		Currency:     "EUR",
		PaymentDate:  ebDay(2026, 9, 7),
		BankID:       "P2",
		Comment:      "Krediidi eel vabastus",
	}); err != nil {
		t.Fatalf("UnallocateToPrepayment: %v", err)
	}
	if _, err := p.UnallocateToPrepayment(ebCtx, UnallocateToPrepaymentInput{
		CustomerCode: "K7224",
		InvoiceNo:    "260124",
		PrepaymentNo: "KLB-CN-10",
		Amount:       d("0.99"),
		PaymentDate:  ebDay(2026, 9, 8),
		BankID:       "P1",
	}); err != nil {
		t.Fatalf("UnallocateToPrepayment minimal: %v", err)
	}
	assertEBWire(t, f,
		`POST IPVc set_field.OKFlag=1&set_field.PayCurCode=EUR&set_field.PayMode=P2&set_field.TransDate=2026-09-07&set_row_field.0.Comment=Krediidi%20eel%20vabastus&set_row_field.0.CustCode=K7224&set_row_field.0.InvoiceNr=260123&set_row_field.0.PayDate=2026-09-07&set_row_field.0.RecVal=-120&set_row_field.0.stp=1&set_row_field.1.CUPNr=KLB-CN-9&set_row_field.1.CustCode=K7224&set_row_field.1.PayDate=2026-09-07&set_row_field.1.RecVal=120&set_row_field.1.stp=1`,
		`POST IPVc set_field.OKFlag=1&set_field.PayMode=P1&set_field.TransDate=2026-09-08&set_row_field.0.CustCode=K7224&set_row_field.0.InvoiceNr=260124&set_row_field.0.PayDate=2026-09-08&set_row_field.0.RecVal=-0.99&set_row_field.0.stp=1&set_row_field.1.CUPNr=KLB-CN-10&set_row_field.1.CustCode=K7224&set_row_field.1.PayDate=2026-09-08&set_row_field.1.RecVal=0.99&set_row_field.1.stp=1`,
	)
}

func TestEBListPrepaymentsWireGolden(t *testing.T) {
	p, f := newEBFake(t, func(int, ebWire) (int, string) { return 200, ebReceiptFixture })
	got, err := p.ListPrepayments(ebCtx, ListPrepaymentsInput{CustomerCode: "K2", Since: ebDay(2026, 1, 1), Until: ebDay(2026, 12, 31)})
	if err != nil {
		t.Fatalf("ListPrepayments: %v", err)
	}
	if len(got) != 1 || got[0].Number != "90000000" || !got[0].Remaining.Equal(d("120")) || !got[0].Amount.Equal(d("120")) {
		t.Fatalf("ListPrepayments(K2) = %+v, want 90000000 120/120", got)
	}
	if _, err := p.ListPrepayments(ebCtx, ListPrepaymentsInput{Since: ebDay(2026, 1, 1)}); err != nil {
		t.Fatalf("ListPrepayments open-ended: %v", err)
	}
	if _, err := p.ListPrepayments(ebCtx, ListPrepaymentsInput{}); err != nil {
		t.Fatalf("ListPrepayments unbounded: %v", err)
	}
	assertEBWire(t, f,
		`GET IPVc?limit=5000&range=2026-01-01%3A2026-12-31&sort=TransDate`,
		`GET IPVc?limit=5000&range=2026-01-01%3A&sort=TransDate`,
		`GET IPVc?limit=5000`,
	)
}

func TestEBDeletePaymentSendsNothing(t *testing.T) {
	p, f := newEBFake(t, nil)
	err := p.DeletePayment(ebCtx, "300001")
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("DeletePayment = %v, want the not-supported error it has always had", err)
	}
	assertEBWire(t, f)
}
