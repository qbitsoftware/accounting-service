package merit

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Merit meters requests per API ID, not per connection. Callers here build a
// fresh Client per operation, so a limiter living on one Client throttles
// nothing the moment two operations for the same company overlap — which is the
// normal case: a scheduled payment pull runs while an admin triggers a manual
// sync and a batch push is still draining, each with its own budget against one
// quota.
//
// Limiters are therefore shared process-wide, keyed by API ID. Note this is
// still per-process: with more than one API replica the quota is split again,
// and a genuinely distributed limiter (Redis) would be needed.
var (
	limiterMu    sync.Mutex
	limiterByKey = map[string]*rate.Limiter{}
)

// sharedLimiter returns the process-wide limiter for an API ID on a host,
// creating it on first use. When an existing limiter is more permissive than the
// caller asks for, it is tightened in place — the most conservative
// configuration seen for a key wins, so one lax caller cannot raise the ceiling
// for everyone else.
//
// The host is part of the key because the quota belongs to a company on a
// specific Merit deployment: the Estonian and Polish services are separate
// budgets, as is any test instance.
func sharedLimiter(host, apiID string, interval time.Duration, burst int) *rate.Limiter {
	limit := rate.Every(interval)

	limiterMu.Lock()
	defer limiterMu.Unlock()

	k := limiterKey(host, apiID)
	l, ok := limiterByKey[k]
	if !ok {
		l = rate.NewLimiter(limit, burst)
		limiterByKey[k] = l
		return l
	}
	if l.Limit() > limit {
		l.SetLimit(limit)
	}
	if l.Burst() > burst {
		l.SetBurst(burst)
	}
	return l
}

// limiterKey hashes host+API ID so the raw credential is not held as a map key.
func limiterKey(host, apiID string) string {
	sum := sha256.Sum256([]byte(host + "\x00" + apiID))
	return hex.EncodeToString(sum[:])
}

// resetSharedLimiters drops all cached limiters. Test-only.
func resetSharedLimiters() {
	limiterMu.Lock()
	defer limiterMu.Unlock()
	limiterByKey = map[string]*rate.Limiter{}
}
