package accounting

import (
	"strings"
	"testing"

	"github.com/qbitsoftware/accounting-service/smartaccounts"
)

// saOriginalByNumber answers the lookup CreateCreditNote makes for the invoice
// it credits, and the re-read auto-settle makes of it.
const saOriginalByNumber = `{"clientInvoices":[{"id":"inv-5","clientId":"cli-9","type":"","invoiceNumber":"LD000877",` +
	`"date":"03.11.2026","currency":"EUR","totalAmount":45,"outstandingAmount":45}],"hasMoreEntries":false}`

// saCreditNoteInput is a credit note as klubio's backend hands it over: Merit's
// convention, every line's quantity negated and its price made positive, so the
// fee is -1 x 50 and the reversed discount -1 x -5.
func saCreditNoteInput() CreateCreditNoteInput {
	return CreateCreditNoteInput{
		CustomerID:        "cli-9",
		CustomerName:      "Mari Maasikas",
		DocDate:           saDay(2026, 11, 3),
		DueDate:           saDay(2026, 11, 3),
		InvoiceNo:         "KRLD000041",
		Currency:          "EUR",
		OriginalInvoiceNo: "LD000877",
		TotalAmount:       d("-45"),
		Lines: []CreateInvoiceLineInput{
			{Code: "00010", Description: "Treeningtasu", Quantity: d("-1"), UnitPrice: d("50"), TaxID: "VABA"},
			{Code: "00010", Description: "Soodustus", Quantity: d("-1"), UnitPrice: d("-5"), TaxID: "VABA"},
		},
	}
}

func saCreditNoteFake(t *testing.T, signed bool, creditTotal string) (*smartProvider, *saFake) {
	t.Helper()
	p, f := newSAFake(t, func(n int, w saWire) (int, string) {
		switch {
		case strings.HasPrefix(w.Path, "purchasesales/clientinvoices:get"):
			return 200, saOriginalByNumber
		case strings.HasPrefix(w.Path, "purchasesales/clientinvoices:add"):
			return 200, `{"invoiceId":"cre-5","clientId":"cli-9","invoiceNumber":"KRLD000041","totalAmount":` + creditTotal + `}`
		}
		return 200, `{"paymentId":"net-5","number":"8","amount":0}`
	})
	p.signedCredits = signed
	return p, f
}

// The wire every club sent before signed credit invoices existed, and every
// club not moved onto them still sends: rows positive, the auto-settle with both
// rows positive and twice the amount. Recorded against v0.16.3 before the change;
// a failure is an existing club's wire changing.
func TestSACreateCreditNoteWireGolden(t *testing.T) {
	p, f := saCreditNoteFake(t, false, "45")
	if _, err := p.CreateCreditNote(saCtx, saCreditNoteInput()); err != nil {
		t.Fatalf("CreateCreditNote: %v", err)
	}
	assertSAWire(t, f, saTestKey(t), saLegacyCreditNoteWire...)
}

var saLegacyCreditNoteWire = []string{
	`GET purchasesales/clientinvoices:get?apikey=KEY&dateFrom=01.01.2000&fetchRows=true&invoiceNumber=LD000877&pageNumber=1`,
	`POST purchasesales/clientinvoices:add?apikey=KEY {"clientId":"cli-9","type":"CRE","baseForCreditInvoiceId":"inv-5","date":"03.11.2026","dueDate":"03.11.2026","invoiceNumber":"KRLD000041","currency":"EUR","rows":[{"code":"00010","description":"Treeningtasu","price":"50","quantity":"1","vatPc":"VABA"},{"code":"00010","description":"Soodustus","price":"-5","quantity":"1","vatPc":"VABA"}]}`,
	`GET purchasesales/clientinvoices:get?apikey=KEY&fetchRows=true&id=inv-5&pageNumber=1`,
	`POST purchasesales/payments:add?apikey=KEY {"date":"03.11.2026","partnerType":"CLIENT","clientId":"cli-9","accountType":"BANK","accountName":"Tasaarveldus","currency":"EUR","amount":"90","comment":"auto-settle credit against original","rows":[{"amount":"45","type":"CLIENT_INVOICE","id":"inv-5"},{"amount":"45","type":"CLIENT_INVOICE","id":"cre-5"}]}`,
}

// A signed credit invoice is the one SmartAccounts makes itself ("Loo
// kreeditarve", recorded 03.11.2026 against a test company): every row carries
// its sign on the quantity with a positive price, so the fee reverses as -1 x 50
// and the reversed discount as +1 x 5, the document totals -45 and lowers the
// customer's balance. Its netting is SmartAccounts' own as well: amount 0, +45 on
// the original and -45 on the credit, through the netting bank.
func TestSACreateSignedCreditNoteWireGolden(t *testing.T) {
	p, f := saCreditNoteFake(t, true, "-45")
	if _, err := p.CreateCreditNote(saCtx, saCreditNoteInput()); err != nil {
		t.Fatalf("CreateCreditNote: %v", err)
	}
	assertSAWire(t, f, saTestKey(t),
		`GET purchasesales/clientinvoices:get?apikey=KEY&dateFrom=01.01.2000&fetchRows=true&invoiceNumber=LD000877&pageNumber=1`,
		`POST purchasesales/clientinvoices:add?apikey=KEY {"clientId":"cli-9","type":"CRE","baseForCreditInvoiceId":"inv-5","date":"03.11.2026","dueDate":"03.11.2026","invoiceNumber":"KRLD000041","currency":"EUR","rows":[{"code":"00010","description":"Treeningtasu","price":"50","quantity":"-1","vatPc":"VABA"},{"code":"00010","description":"Soodustus","price":"5","quantity":"1","vatPc":"VABA"}]}`,
		`GET purchasesales/clientinvoices:get?apikey=KEY&fetchRows=true&id=inv-5&pageNumber=1`,
		`POST purchasesales/payments:add?apikey=KEY {"date":"03.11.2026","partnerType":"CLIENT","clientId":"cli-9","accountType":"BANK","accountName":"Tasaarveldus","currency":"EUR","amount":"0","comment":"auto-settle credit against original","rows":[{"amount":"45","type":"CLIENT_INVOICE","id":"inv-5"},{"amount":"-45","type":"CLIENT_INVOICE","id":"cre-5"}]}`,
	)
}

// The switch is read from the provider config: absent is the old wire, so a
// caller that never heard of it keeps sending what it always sent.
func TestSASignedCreditsIsOptIn(t *testing.T) {
	if newSmartAccountsProvider(Config{Extra: map[string]string{}}).signedCredits {
		t.Fatal("signed credit invoices are on without being asked for")
	}
	if !newSmartAccountsProvider(Config{Extra: map[string]string{SmartAccountsCreditRows: SmartAccountsCreditRowsSigned}}).signedCredits {
		t.Fatal("signed credit invoices are off although asked for")
	}
}

func TestSASignedCreditRows(t *testing.T) {
	rows := saSignedCreditRows([]smartaccounts.InvoiceRowInput{
		{Price: d("50"), Quantity: d("-1")},   // the fee reversed
		{Price: d("-5"), Quantity: d("-1")},   // a discount reversed
		{Price: d("-12.5"), Quantity: d("2")}, // already negative the other way round
		{Price: d("0"), Quantity: d("-3")},    // nothing to sign
	})
	want := [][2]string{{"50", "-1"}, {"5", "1"}, {"12.5", "-2"}, {"0", "-3"}}
	for i, r := range rows {
		if r.Price.String() != want[i][0] || r.Quantity.String() != want[i][1] {
			t.Errorf("row %d = %s x %s, want %s x %s", i, r.Quantity, r.Price, want[i][1], want[i][0])
		}
	}
}
