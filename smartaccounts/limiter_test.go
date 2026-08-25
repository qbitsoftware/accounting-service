package smartaccounts

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// Two clients for the same company must share one bucket — SmartAccounts meters
// per apikey, and clients are constructed per operation.
func TestSharedLimiterPerAPIKey(t *testing.T) {
	resetSharedLimiters()
	t.Cleanup(resetSharedLimiters)

	a := New(Config{APIKey: "company-a", SecretKey: "s"})
	b := New(Config{APIKey: "company-a", SecretKey: "s"})
	if a.limiter != b.limiter {
		t.Error("clients with the same API key should share a limiter")
	}

	c := New(Config{APIKey: "company-b", SecretKey: "s"})
	if c.limiter == a.limiter {
		t.Error("clients with different API keys must not share a limiter")
	}
}

// A sandbox/test host is a separate quota and must not throttle production.
func TestSharedLimiterSeparatesHosts(t *testing.T) {
	resetSharedLimiters()
	t.Cleanup(resetSharedLimiters)

	prod := New(Config{APIKey: "company-a", SecretKey: "s"})
	other := New(Config{Host: "sandbox.example.test", APIKey: "company-a", SecretKey: "s"})
	if prod.limiter == other.limiter {
		t.Error("same key on a different host should get its own limiter")
	}
}

// The most conservative configuration wins: a later, laxer client must not raise
// the ceiling for one that asked to be throttled harder.
func TestSharedLimiterTakesMostConservative(t *testing.T) {
	resetSharedLimiters()
	t.Cleanup(resetSharedLimiters)

	strict := New(Config{APIKey: "company-a", SecretKey: "s", RequestInterval: 5 * time.Second, Burst: 1})
	lax := New(Config{APIKey: "company-a", SecretKey: "s", RatePerSecond: 10, Burst: 50})

	if lax.limiter != strict.limiter {
		t.Fatal("same key should share a limiter regardless of config")
	}
	if got, want := strict.limiter.Limit(), rate.Every(5*time.Second); got != want {
		t.Errorf("limit loosened by later client: got %v, want %v", got, want)
	}
	if got := strict.limiter.Burst(); got != 1 {
		t.Errorf("burst loosened by later client: got %d, want 1", got)
	}
}

// A stricter client arriving second must tighten the existing bucket.
func TestSharedLimiterTightensExisting(t *testing.T) {
	resetSharedLimiters()
	t.Cleanup(resetSharedLimiters)

	first := New(Config{APIKey: "company-a", SecretKey: "s"})
	New(Config{APIKey: "company-a", SecretKey: "s", RequestInterval: 10 * time.Second, Burst: 1})

	if got, want := first.limiter.Limit(), rate.Every(10*time.Second); got != want {
		t.Errorf("existing limiter not tightened: got %v, want %v", got, want)
	}
	if got := first.limiter.Burst(); got != 1 {
		t.Errorf("existing burst not tightened: got %d, want 1", got)
	}
}

func TestThrottlingCanBeDisabled(t *testing.T) {
	resetSharedLimiters()
	t.Cleanup(resetSharedLimiters)

	c := New(Config{APIKey: "company-a", SecretKey: "s", RatePerSecond: -1})
	if c.limiter != nil {
		t.Error("RatePerSecond < 0 should disable throttling entirely")
	}
}

func TestDefaultRateStaysUnderPerMinuteCap(t *testing.T) {
	resetSharedLimiters()
	t.Cleanup(resetSharedLimiters)

	c := New(Config{APIKey: "company-a", SecretKey: "s"})
	perMin := float64(c.limiter.Limit()) * 60
	if perMin >= 60 {
		t.Errorf("default sustained rate %.1f req/min is not below SmartAccounts' 60/min cap", perMin)
	}
}
