package accounting

import (
	"testing"

	"github.com/shopspring/decimal"
)

func sbd(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func sbLine(qty, price, rate string) CreateInvoiceLineInput {
	return CreateInvoiceLineInput{Quantity: d(qty), UnitPrice: d(price), TaxRate: d(rate)}
}

// Verified live against SimplBooks: two rows of 10.02 at 24% come back as
// net 20.04, VAT 4.81, total 24.85 — SimplBooks taxes the DOCUMENT's net once.
// klubio rounds each row's VAT to 2.40 and bills 24.84. The rounding line is
// what makes the two documents agree without either changing its arithmetic.
func TestRoundingClosesTheDocumentGap(t *testing.T) {
	lines := []CreateInvoiceLineInput{sbLine("1", "10.02", "24"), sbLine("1", "10.02", "24")}
	got, ok := simplbooksRounding(lines, sbd("24.84"))
	if !ok {
		t.Fatal("a one-cent gap must be closed with a rounding line")
	}
	if want := sbd("-0.01"); !got.Equal(want) {
		t.Fatalf("rounding = %s, want %s", got, want)
	}
}

// The common case: the two arithmetics already agree and nothing is sent.
func TestNoRoundingWhenTheTotalsAlreadyAgree(t *testing.T) {
	// Tennis One INV000023 — verified live as 260.00 on both sides.
	lines := []CreateInvoiceLineInput{sbLine("1", "-8.0645", "24"), sbLine("1", "217.7419", "24")}
	if _, ok := simplbooksRounding(lines, sbd("260.00")); ok {
		t.Fatal("an agreeing document must not carry a rounding line")
	}
	if _, ok := simplbooksRounding(lines, decimal.Zero); ok {
		t.Fatal("a caller that stated no total must not get a rounding line")
	}
}

// A gap bigger than rounding can explain is a real disagreement about what the
// invoice says. Hiding it under a rounding line is the one outcome worse than
// the cent — the read-back check reports it instead.
func TestLargeGapIsNotPaperedOver(t *testing.T) {
	lines := []CreateInvoiceLineInput{sbLine("1", "10.00", "24")}
	if _, ok := simplbooksRounding(lines, sbd("99.99")); ok {
		t.Fatal("a gap this size is not rounding and must not be absorbed")
	}
	// SimplBooks would total these two rows at 24.85. The bound is one cent per
	// line plus one for the document, so a drift of up to three cents is still
	// rounding and a fourth is not.
	two := []CreateInvoiceLineInput{sbLine("1", "10.02", "24"), sbLine("1", "10.02", "24")}
	if _, ok := simplbooksRounding(two, sbd("24.88")); !ok {
		t.Fatal("three cents over two lines is still rounding")
	}
	if _, ok := simplbooksRounding(two, sbd("24.89")); ok {
		t.Fatal("four cents over two lines is beyond what rounding explains")
	}
}

// A document mixing rates is taxed per rate by SimplBooks, so the prediction has
// to group before it rounds — summing everything and taxing once would invent a
// gap that is not there.
func TestMixedRatesArePredictedPerRate(t *testing.T) {
	lines := []CreateInvoiceLineInput{sbLine("1", "100.00", "24"), sbLine("1", "100.00", "9")}
	// 100.00 + 24.00 + 100.00 + 9.00 = 233.00
	if _, ok := simplbooksRounding(lines, sbd("233.00")); ok {
		t.Fatal("a correctly predicted mixed-rate document needs no rounding")
	}
}

func TestZeroRatedLinesPredictCleanly(t *testing.T) {
	lines := []CreateInvoiceLineInput{sbLine("1", "50.00", "0")}
	if _, ok := simplbooksRounding(lines, sbd("50.00")); ok {
		t.Fatal("a 0% document needs no rounding")
	}
}
