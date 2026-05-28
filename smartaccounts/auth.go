package smartaccounts

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// tallinnTZ is Europe/Tallinn resolved from system tzdata, or nil when the
// container ships no zoneinfo (scratch/distroless). When nil we compute the
// offset from the EU DST rule per-request — see estonianNow.
var tallinnTZ, _ = time.LoadLocation("Europe/Tallinn")

// estonianNow returns the current time in Estonian local time. SmartAccounts
// rejects requests whose timestamp is more than 15 minutes off server time, so
// the offset must be correct year-round including summer DST (Estonia is UTC+2
// in winter, UTC+3 in summer). A fixed-offset fallback would be an hour wrong
// for ~7 months a year on tzdata-less images, so we derive the offset from the
// EU DST schedule instead.
func estonianNow() time.Time {
	now := time.Now()
	if tallinnTZ != nil {
		return now.In(tallinnTZ)
	}
	offset := 2 * 60 * 60 // EET (winter)
	if isEUSummerTime(now.UTC()) {
		offset = 3 * 60 * 60 // EEST (summer)
	}
	return now.In(time.FixedZone("EE", offset))
}

// isEUSummerTime reports whether the given UTC instant falls within EU summer
// time: 01:00 UTC on the last Sunday of March until 01:00 UTC on the last
// Sunday of October.
func isEUSummerTime(utc time.Time) bool {
	year := utc.Year()
	start := lastSundayAt01UTC(year, time.March)
	end := lastSundayAt01UTC(year, time.October)
	return !utc.Before(start) && utc.Before(end)
}

// lastSundayAt01UTC returns 01:00 UTC on the last Sunday of the given month.
func lastSundayAt01UTC(year int, month time.Month) time.Time {
	// Day 0 of the next month == last day of this month.
	last := time.Date(year, month+1, 0, 1, 0, 0, 0, time.UTC)
	return last.AddDate(0, 0, -int(last.Weekday()))
}

// timestamp returns the current Estonian local time formatted as the
// SmartAccounts request timestamp: ddMMyyyyHHmmss.
func timestamp() string {
	return estonianNow().Format("02012006150405")
}

// sign computes the HMAC-SHA256 signature for a SmartAccounts request and
// returns it hex-encoded.
//
// The signed message is the request's query string (everything after "?" up
// to, but not including, the "signature" parameter) immediately followed by
// the raw request body with no separator. The secret key is the HMAC key.
// This mirrors the reference Postman pre-request script.
func sign(secretKey, signedInput string) string {
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(signedInput))
	return hex.EncodeToString(mac.Sum(nil))
}
