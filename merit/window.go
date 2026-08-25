package merit

import (
	"fmt"
	"strings"
	"time"
)

// DateFormat is the layout Merit uses for every date parameter (YYYYMMDD).
const DateFormat = "20060102"

// FormatDate renders t in Merit's date format. A zero time yields "", which
// Merit's optional date parameters treat as unset.
func FormatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(DateFormat)
}

// MaxPeriodDays is the widest period a single Merit list call accepts.
//
// Merit documents the limit as "3 months" and answers anything wider with
// HTTP 400 "Periood liiga pikk(max 3 kuud)". Three calendar months is as short
// as 89 days (1 December → 1 March in a non-leap year), so a literal 90-day
// slice sits right on the boundary for some start dates and over it for others.
// 85 days clears it for every month combination with room to spare, at a cost
// of roughly one extra request per year of history.
const MaxPeriodDays = 85

// Period is a closed date range [Start, End]. Merit's period parameters are
// date-granular and inclusive at both ends.
type Period struct {
	Start time.Time
	End   time.Time
}

func (p Period) String() string {
	return fmt.Sprintf("%s..%s", FormatDate(p.Start), FormatDate(p.End))
}

// SplitPeriod divides [start, end] into consecutive, non-overlapping periods of
// at most MaxPeriodDays each, so an arbitrarily long range can be fetched as a
// series of legal calls. Slices are returned oldest first; the last one absorbs
// the remainder rather than overshooting end.
//
// This is what makes history reachable at all: the 3-month cap is a limit on
// one *call*, not on how far back Merit will look, so any invoice from any year
// can be fetched by asking for the window it falls in.
//
// A zero or inverted range yields no periods. A range already within the limit
// yields exactly one, identical to the input.
func SplitPeriod(start, end time.Time) []Period {
	return splitPeriod(start, end, MaxPeriodDays)
}

func splitPeriod(start, end time.Time, maxDays int) []Period {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return nil
	}
	if maxDays < 1 {
		maxDays = 1
	}

	// Work in whole days: Merit ignores the time of day, and keeping the
	// boundaries on date granularity is what guarantees the slices tile the
	// range exactly — no gap that would silently drop a day's invoices, no
	// overlap that would fetch one twice.
	start = truncateDay(start)
	end = truncateDay(end)

	var periods []Period
	for cursor := start; !cursor.After(end); {
		// -1 because the range is inclusive at both ends: a maxDays-day period
		// starting on day 0 ends on day maxDays-1.
		chunkEnd := cursor.AddDate(0, 0, maxDays-1)
		if chunkEnd.After(end) {
			chunkEnd = end
		}
		periods = append(periods, Period{Start: cursor, End: chunkEnd})
		cursor = chunkEnd.AddDate(0, 0, 1)
	}
	return periods
}

func truncateDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// IsPeriodTooLong reports whether err is Merit rejecting a list call because the
// requested period exceeded its 3-month cap. Merit answers with a 400 and a
// localized message rather than a machine-readable code, so this matches the
// Estonian and Polish wording. Callers should prefer SplitPeriod over reacting
// to this — it exists so a caller that built its own window can tell this apart
// from a genuine bad request.
func IsPeriodTooLong(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "liiga pikk") || // Estonian
		strings.Contains(msg, "zbyt d") // Polish "zbyt długi/długa"
}
