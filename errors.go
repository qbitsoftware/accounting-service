package accounting

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrAuthFailed          = errors.New("authentication failed")
	ErrRateLimit           = errors.New("rate limit exceeded")
	ErrUnsupportedProvider = errors.New("unsupported provider")
	ErrInvalidInput        = errors.New("invalid input")
)

// ProviderError wraps an error with provider and operation context.
type ProviderError struct {
	Provider string
	Op       string
	Err      error
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("%s: %s: %v", e.Provider, e.Op, e.Err)
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

func IsAuthFailed(err error) bool {
	return errors.Is(err, ErrAuthFailed)
}

func IsRateLimit(err error) bool {
	return errors.Is(err, ErrRateLimit)
}

// PartialResultError is implemented by errors that accompany a usable partial
// result. It exists because some providers cap how much history one call may
// cover (Merit: 3 months per list call), so a wide range is fetched as several
// calls and one of them failing does not invalidate the rest.
//
// Providers report it rather than swallowing it: a caller reconciling payments
// needs to know it saw 11 windows of 12, because "no payment found" and "the
// window that held it failed" mean opposite things.
type PartialResultError interface {
	error
	// PartialResult reports how many sub-requests failed out of how many the
	// range was split into.
	PartialResult() (failed, total int)
}

// IsPartialResult reports whether err carries a usable partial result, and how
// much of the range was missed. Results returned alongside such an error are
// safe to use; they are simply incomplete.
func IsPartialResult(err error) (failed, total int, ok bool) {
	var p PartialResultError
	if errors.As(err, &p) {
		failed, total = p.PartialResult()
		return failed, total, true
	}
	return 0, 0, false
}

// IsCustomerExistsError reports whether err is a Merit "customer already
// exists" error. Merit returns these as plain-text body containing
// "custexists" rather than a structured status — string matching is the
// only reliable signal today.
func IsCustomerExistsError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "custexists")
}

// IsDuplicateInvoiceError reports whether err signals that the provider
// already has an invoice with the same number. Merit returns "Korduv arve"
// (Estonian); other providers may surface English variants — we match both.
func IsDuplicateInvoiceError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "korduv arve") || strings.Contains(msg, "duplicate invoice")
}
