package accounting

import (
	"errors"
	"testing"

	"github.com/qbitsoftware/accounting-service/merit"
)

// The backend reconciles payments across a year of history, which Merit serves
// as several calls. It has to be able to tell "nothing to apply" from "the
// window that held it failed" — through the ProviderError the adapter wraps
// every provider error in.
func TestIsPartialResultThroughProviderError(t *testing.T) {
	inner := &merit.PartialRangeError{
		Failed: []merit.PeriodError{{Err: errors.New("timeout")}},
		Total:  12,
	}
	wrapped := &ProviderError{Provider: "merit", Op: "ListPayments", Err: inner}

	failed, total, ok := IsPartialResult(wrapped)
	if !ok {
		t.Fatal("partial result not detected through ProviderError")
	}
	if failed != 1 || total != 12 {
		t.Errorf("got %d of %d, want 1 of 12", failed, total)
	}

	if _, _, ok := IsPartialResult(errors.New("plain")); ok {
		t.Error("a plain error is not a partial result")
	}
	if _, _, ok := IsPartialResult(nil); ok {
		t.Error("nil is not a partial result")
	}
}
