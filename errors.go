package accounting

import (
	"errors"
	"fmt"
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
