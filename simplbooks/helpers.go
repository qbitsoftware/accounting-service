package simplbooks

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// decodeBase64 decodes a base64 payload, tolerating surrounding whitespace and
// both standard and URL-safe alphabets.
func decodeBase64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("simplbooks: decode base64: %w", err)
	}
	return b, nil
}
