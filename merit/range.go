package merit

import (
	"context"
	"fmt"
	"strings"
)

// PartialRangeError reports that some — but not all — periods of a multi-call
// range fetch failed. The results returned alongside it are the ones that did
// succeed and are safe to use.
//
// A range walk deliberately does not abandon everything because one slice
// failed: a payment pull covering a year is 5 calls, and losing the whole run
// because a single window timed out is how a club silently stops reconciling.
// Callers that need all-or-nothing check for this error; callers that prefer
// progress log it and use what came back.
type PartialRangeError struct {
	Failed []PeriodError
	// Total is how many periods the range was split into, so a caller can tell
	// "one window of twelve" from "eleven of twelve".
	Total int
}

// PeriodError pairs a failed period with the reason it failed.
type PeriodError struct {
	Period Period
	Err    error
}

func (e *PartialRangeError) Error() string {
	parts := make([]string, 0, len(e.Failed))
	for _, f := range e.Failed {
		parts = append(parts, fmt.Sprintf("%s: %v", f.Period, f.Err))
	}
	return fmt.Sprintf("merit: %d of %d periods failed: %s",
		len(e.Failed), e.Total, strings.Join(parts, "; "))
}

// PartialResult implements accounting.PartialResultError so callers can detect
// an incomplete range without importing this package.
func (e *PartialRangeError) PartialResult() (failed, total int) {
	return len(e.Failed), e.Total
}

// Unwrap returns the first failure so errors.Is/As can reach the underlying
// API error (auth failures in particular, which are the same for every period
// and which callers translate into a "check your credentials" message).
func (e *PartialRangeError) Unwrap() error {
	if len(e.Failed) == 0 {
		return nil
	}
	return e.Failed[0].Err
}

// fetchRange runs fetch over every period the range splits into and returns the
// concatenated results. Every period is attempted even when an earlier one
// failed; if any failed, the results so far are returned together with a
// *PartialRangeError. When *all* periods fail the error is returned with no
// results, since there is nothing partial about it.
func fetchRange[T any](
	ctx context.Context,
	periods []Period,
	fetch func(context.Context, Period) ([]T, error),
) ([]T, error) {
	var out []T
	var failed []PeriodError

	for _, p := range periods {
		// A cancelled context is not a per-period failure to be collected and
		// carried on from — every remaining call would fail the same way.
		if err := ctx.Err(); err != nil {
			failed = append(failed, PeriodError{Period: p, Err: err})
			break
		}
		items, err := fetch(ctx, p)
		if err != nil {
			failed = append(failed, PeriodError{Period: p, Err: err})
			continue
		}
		out = append(out, items...)
	}

	if len(failed) == 0 {
		return out, nil
	}
	err := &PartialRangeError{Failed: failed, Total: len(periods)}
	if len(failed) == len(periods) {
		return nil, err
	}
	return out, err
}

// dedupeBy drops later items whose key repeats one already seen, preserving
// order. Range slices are built not to overlap, so this only ever fires if
// Merit treats a period boundary as inclusive on both sides — cheap insurance
// against booking the same payment twice.
func dedupeBy[T any](items []T, key func(T) string) []T {
	seen := make(map[string]struct{}, len(items))
	out := items[:0]
	for _, it := range items {
		k := key(it)
		if k != "" {
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
		}
		out = append(out, it)
	}
	return out
}
