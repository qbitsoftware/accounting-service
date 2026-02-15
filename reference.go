package accounting

import (
	"fmt"
	"strconv"
	"strings"
)

// GenerateEstonianReference generates an Estonian bank reference number using the 3-7-1 algorithm.
// This reference number (viitenumber) is used for automatic payment matching in Estonian banking.
//
// The algorithm:
// 1. Takes a base number (invoice number, customer ID, etc.)
// 2. Multiplies each digit by weights [7, 3, 1] repeating from right to left
// 3. Sums all products
// 4. Calculates check digit: (10 - (sum mod 10)) mod 10
// 5. Appends check digit to the base number
//
// Example:
//   GenerateEstonianReference("1234") returns "12347"
//   GenerateEstonianReference("100001") returns "1000016"
func GenerateEstonianReference(base string) (string, error) {
	// Remove any whitespace
	base = strings.TrimSpace(base)

	// Validate base is not empty
	if base == "" {
		return "", fmt.Errorf("base number cannot be empty")
	}

	// Validate base contains only digits
	for _, ch := range base {
		if ch < '0' || ch > '9' {
			return "", fmt.Errorf("base number must contain only digits, got: %s", base)
		}
	}

	// Calculate check digit using 3-7-1 weights
	checkDigit := calculateEstonian371CheckDigit(base)

	return base + strconv.Itoa(checkDigit), nil
}

// calculateEstonian371CheckDigit calculates the check digit for Estonian reference numbers.
// Weights are [7, 3, 1] applied from right to left.
func calculateEstonian371CheckDigit(base string) int {
	weights := []int{7, 3, 1}
	sum := 0

	// Process digits from right to left
	for i := len(base) - 1; i >= 0; i-- {
		digit := int(base[i] - '0')
		weight := weights[(len(base)-1-i)%3]
		sum += digit * weight
	}

	// Check digit formula: (10 - (sum mod 10)) mod 10
	checkDigit := (10 - (sum % 10)) % 10
	return checkDigit
}

// ValidateEstonianReference validates an Estonian reference number.
// Returns true if the reference number has a valid check digit.
func ValidateEstonianReference(reference string) bool {
	reference = strings.TrimSpace(reference)

	// Must be at least 2 digits (base + check digit)
	if len(reference) < 2 {
		return false
	}

	// Must contain only digits
	for _, ch := range reference {
		if ch < '0' || ch > '9' {
			return false
		}
	}

	// Split into base and check digit
	base := reference[:len(reference)-1]
	providedCheckDigit := int(reference[len(reference)-1] - '0')

	// Calculate expected check digit
	expectedCheckDigit := calculateEstonian371CheckDigit(base)

	return providedCheckDigit == expectedCheckDigit
}
