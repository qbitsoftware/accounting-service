package smartaccounts

import (
	"testing"
	"time"
)

func TestDayBudgetCountsOnlyTheTrailing24Hours(t *testing.T) {
	resetSharedBudgets()
	b := &dayBudget{}

	now := time.Now()
	// Yesterday's spend has aged out; today's has not.
	for i := 0; i < 5; i++ {
		b.record(now.Add(-25 * time.Hour))
	}
	var used int
	for i := 0; i < 3; i++ {
		used = b.record(now)
	}
	if used != 3 {
		t.Fatalf("expected the 25-hour-old requests to age out, got used=%d", used)
	}
}

func TestDayBudgetWarnsOncePerDecile(t *testing.T) {
	b := &dayBudget{}
	now := time.Now()

	warnings := 0
	for i := 0; i < DailyRequestLimit; i++ {
		if d := b.warnThreshold(b.record(now)); d > 0 {
			warnings++
		}
	}
	// 75% is the floor, so deciles 7, 8, 9 and 10 report — four lines for a
	// full budget, not one per request.
	if warnings != 4 {
		t.Fatalf("expected 4 threshold crossings over a full budget, got %d", warnings)
	}
}

func TestDayBudgetStopsGrowingPastTheCap(t *testing.T) {
	b := &dayBudget{}
	now := time.Now()
	for i := 0; i < DailyRequestLimit*2; i++ {
		b.record(now)
	}
	if got := len(b.sent); got != DailyRequestLimit {
		t.Fatalf("budget slice should stop at the cap, got %d entries", got)
	}
}

func TestBudgetIsSharedPerCompanyNotPerClient(t *testing.T) {
	resetSharedBudgets()
	// Two clients for one company — the normal case, since callers build a
	// fresh client per operation.
	a := New(Config{APIKey: "k", SecretKey: "s", RatePerSecond: -1})
	c := New(Config{APIKey: "k", SecretKey: "s", RatePerSecond: -1})
	if a.budget != c.budget {
		t.Fatal("two clients for the same company must share one budget meter")
	}
	other := New(Config{APIKey: "other", SecretKey: "s", RatePerSecond: -1})
	if other.budget == a.budget {
		t.Fatal("a different company must have its own budget meter")
	}
}
