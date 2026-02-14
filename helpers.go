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
