package accounting

import (
	"fmt"
	"sort"
	"strings"
)

// NormalizeCustomerName normalizes a customer name for fuzzy comparison.
// Lowercases, replaces separators (hyphens, dots, commas) with spaces,
// splits into tokens, and sorts them alphabetically. This makes the
// following comparisons true:
//
//   - "Jaan-Erik" == "Jaan Erik"   (separator difference)
//   - "OÜ Testfirma" == "Testfirma OÜ"   (word order difference)
//
// Returns "" for input that contains no tokens.
func NormalizeCustomerName(s string) string {
	s = strings.ToLower(s)
	s = strings.NewReplacer("-", " ", ".", " ", ",", " ").Replace(s)
	tokens := strings.Fields(s)
	if len(tokens) == 0 {
		return ""
	}
	sort.Strings(tokens)
	return strings.Join(tokens, " ")
}

// MatchCustomerFromList searches the slice for the first customer matching
// by RegNo, email, or normalized name (in that order of preference).
// Returns an error if none match.
//
// Used after a FindOrCreate fails with IsCustomerExistsError: the customer
// is known to exist on the provider side but couldn't be located by the
// initial email lookup — so callers list all customers and call this to
// fish out the right one.
//
// Matching rules:
//   - RegNo: exact match after TrimSpace
//   - Email: case-insensitive match after TrimSpace
//   - Name: equality after NormalizeCustomerName on both sides
//
// Empty fields are skipped (an empty RegNo doesn't match an empty RegNo).
func MatchCustomerFromList(customers []Customer, name, regNo, email string) (*Customer, error) {
	normalizedName := NormalizeCustomerName(name)
	regNo = strings.TrimSpace(regNo)
	email = strings.ToLower(strings.TrimSpace(email))

	for i := range customers {
		c := &customers[i]
		if regNo != "" && strings.TrimSpace(c.RegNo) == regNo {
			return c, nil
		}
		if email != "" && strings.ToLower(strings.TrimSpace(c.Email)) == email {
			return c, nil
		}
		if normalizedName != "" && NormalizeCustomerName(c.Name) == normalizedName {
			return c, nil
		}
	}
	return nil, fmt.Errorf("customer exists in provider but could not be matched (name=%s, regNo=%s, email=%s)", name, regNo, email)
}
