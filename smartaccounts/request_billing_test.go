package smartaccounts

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
)

// A billing-error 503 must come back on the first answer: retrying it held a
// connection test past every caller's timeout.
func TestBillingErrorIsNotRetried(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"errors":[{"code":"GENERAL","message":"Service unavailable (billing error)"}]}`))
	})
	if err := c.get(context.Background(), "settings/bankaccounts:get", nil, nil); err == nil {
		t.Fatal("expected an error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("billing error was sent %d times, want 1", got)
	}
}
