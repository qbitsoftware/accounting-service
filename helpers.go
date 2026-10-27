package accounting

import "time"

const meritDateFormat = "20060102"

func parseDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(meritDateFormat, s)
	return t
}

// parseMeritAnyDate reads a date in any of the shapes Merit answers with —
// yyyymmdd on some endpoints, "2026-09-29T00:00:00" on getpayments and a
// Z-suffixed form on getinvoice's receipts. Zero when none fits.
func parseMeritAnyDate(s string) time.Time {
	for _, layout := range []string{
		meritDateFormat,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.999999999",
		time.RFC3339,
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(meritDateFormat)
}

func intPtr(i int) *int {
	return &i
}

func stringPtr(s string) *string {
	return &s
}
