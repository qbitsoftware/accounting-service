package merit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"time"
)

// timestamp returns the current UTC time formatted as YYYYMMDDHHmmss.
func timestamp() string {
	return time.Now().UTC().Format("20060102150405")
}

// sign computes the HMAC-SHA256 signature for a Merit API request.
// The data to sign is the concatenation of apiID + timestamp + body (as UTF-8 bytes).
// The API key is used as the HMAC key (as ASCII bytes).
// Returns the base64-encoded signature.
func sign(apiID, apiKey, ts, body string) string {
	data := []byte(apiID + ts + body)
	key := []byte(apiKey)

	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	sig := mac.Sum(nil)

	return base64.StdEncoding.EncodeToString(sig)
}

// urlEncodeSignature URL-encodes a base64 signature for use in query parameters.
func urlEncodeSignature(sig string) string {
	return url.QueryEscape(sig)
}
