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
