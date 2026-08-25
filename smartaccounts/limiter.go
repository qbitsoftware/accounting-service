package smartaccounts

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// SmartAccounts meters requests per company (apikey), not per connection, so a
// limiter that lives on one Client throttles nothing when a second Client is
// built for the same company — which is the normal case here: callers construct
// a fresh client per operation, so a scheduled pull overlapping an invoice push
// ran two independent budgets against one quota and blew the cap.
//
// Limiters are therefore shared process-wide, keyed by API key. Note this is
// still per-process: with more than one API replica the quota is split again,
// and a genuinely distributed limiter (Redis) would be needed.
var (
	limiterMu    sync.Mutex
	limiterByKey = map[string]*rate.Limiter{}
)

// sharedLimiter returns the process-wide limiter for a company on a host,
// creating it on first use. When an existing limiter is more permissive than the
// caller asks for, it is tightened in place — the most conservative
// configuration seen for a key wins, so one lax caller cannot raise the ceiling
// for everyone else.
//
// The host is part of the key because the quota belongs to a company on a
// specific SmartAccounts deployment: a sandbox (or a test server) is a separate
// budget from production and must not throttle it.
func sharedLimiter(host, apiKey string, interval time.Duration, burst int) *rate.Limiter {
	limit := rate.Every(interval)

	limiterMu.Lock()
	defer limiterMu.Unlock()

	k := limiterKey(host, apiKey)
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

// limiterKey hashes host+API key so the raw credential is not held as a map key.
func limiterKey(host, apiKey string) string {
	sum := sha256.Sum256([]byte(host + "\x00" + apiKey))
	return hex.EncodeToString(sum[:])
}

// resetSharedLimiters drops all cached limiters. Test-only.
func resetSharedLimiters() {
	limiterMu.Lock()
	defer limiterMu.Unlock()
	limiterByKey = map[string]*rate.Limiter{}
}
