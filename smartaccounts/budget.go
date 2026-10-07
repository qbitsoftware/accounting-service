package smartaccounts

import (
	"log/slog"
	"sync"
	"time"
)

// DailyRequestLimit is SmartAccounts' per-company cap on requests in any 24
// hour window (SmartAccounts_API_latest.pdf, § Request limits). Unlike the
// 60/minute cap it cannot be smoothed away by pacing — it is a budget, and how
// it is spent is the caller's design problem.
const DailyRequestLimit = 1000

// dailyWarnAt is the share of the budget at which a company's usage starts
// being reported. A club whose day's budget runs out sees only "rate limit
// exceeded" on whatever request happens to be next, which is never the request
// that spent the budget — so the useful signal has to be emitted on the way up.
const dailyWarnAt = 0.75

var (
	budgetMu    sync.Mutex
	budgetByKey = map[string]*dayBudget{}
)

// dayBudget counts requests sent for one company within a rolling 24 hours.
//
// It is an observability aid, not a limiter: it never blocks a request, and it
// is deliberately not used to pre-emptively fail one. It also counts only this
// process's requests — like the shared limiter, with several API replicas the
// real usage is the sum across them, and a restart starts the count at zero. So
// treat a reported figure as a floor: "at least this much of the budget is
// gone".
type dayBudget struct {
	mu       sync.Mutex
	sent     []time.Time // ascending; pruned to the trailing 24h
	warnedAt int         // highest decile already reported, so one crossing logs once
}

func sharedBudget(host, apiKey string) *dayBudget {
	k := limiterKey(host, apiKey)
	budgetMu.Lock()
	defer budgetMu.Unlock()
	b, ok := budgetByKey[k]
	if !ok {
		b = &dayBudget{}
		budgetByKey[k] = b
	}
	return b
}

// record notes one request against the budget and returns how many are now
// known to have been sent in the trailing 24 hours.
func (b *dayBudget) record(now time.Time) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	cutoff := now.Add(-24 * time.Hour)
	keep := 0
	for keep < len(b.sent) && b.sent[keep].Before(cutoff) {
		keep++
	}
	b.sent = append(b.sent[:0], b.sent[keep:]...)

	// Past the cap the exact figure stops mattering and the slice would grow
	// without bound on a runaway caller, so stop recording individual requests
	// and keep reporting the ceiling.
	if len(b.sent) < DailyRequestLimit {
		b.sent = append(b.sent, now)
	}
	return len(b.sent)
}

// warnThreshold returns the decile of the budget that used crosses, or 0 when
// usage is still below dailyWarnAt or has already been reported at that decile.
func (b *dayBudget) warnThreshold(used int) int {
	if used < int(dailyWarnAt*float64(DailyRequestLimit)) {
		return 0
	}
	decile := used * 10 / DailyRequestLimit
	b.mu.Lock()
	defer b.mu.Unlock()
	if decile <= b.warnedAt {
		return 0
	}
	b.warnedAt = decile
	return decile
}

// noteRequest records a request against the company's 24h budget and reports
// usage once per decile past dailyWarnAt.
func (c *Client) noteRequest(endpoint string) {
	if c.budget == nil {
		return
	}
	used := c.budget.record(time.Now())
	if d := c.budget.warnThreshold(used); d > 0 {
		slog.Warn("smartaccounts: approaching the 24h request budget",
			"used_at_least", used,
			"limit", DailyRequestLimit,
			"endpoint", endpoint,
			"note", "count is this process only — with several replicas real usage is higher")
	}
}

// resetSharedBudgets drops all cached budgets. Test-only.
func resetSharedBudgets() {
	budgetMu.Lock()
	defer budgetMu.Unlock()
	budgetByKey = map[string]*dayBudget{}
}

// used returns how many requests are recorded in the trailing 24 hours,
// without recording one.
func (b *dayBudget) used(now time.Time) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	cutoff := now.Add(-24 * time.Hour)
	n := 0
	for _, t := range b.sent {
		if !t.Before(cutoff) {
			n++
		}
	}
	return n
}
