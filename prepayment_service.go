package accounting

import "context"

// PrepaymentService exposes customer-prepayment (ettemaks) operations. The
// underlying provider must implement the optional PrepaymentProvider capability;
// if it does not, every method returns ErrUnsupportedProvider wrapped with the
// provider name. Callers can probe support with Supported().
type PrepaymentService struct {
	provider     Provider
	providerName string
}

// Supported reports whether the configured provider implements prepayments.
func (s *PrepaymentService) Supported() bool {
	_, ok := s.provider.(PrepaymentProvider)
	return ok
}

func (s *PrepaymentService) capable(op string) (PrepaymentProvider, error) {
	pp, ok := s.provider.(PrepaymentProvider)
	if !ok {
		return nil, &ProviderError{Provider: s.providerName, Op: op, Err: ErrUnsupportedProvider}
	}
	return pp, nil
}

// Create records an unallocated customer advance.
func (s *PrepaymentService) Create(ctx context.Context, input CreatePrepaymentInput) (*Prepayment, error) {
	pp, err := s.capable("CreatePrepayment")
	if err != nil {
		return nil, err
	}
	return pp.CreatePrepayment(ctx, input)
}

// Apply settles (part of) an invoice from an existing prepayment.
func (s *PrepaymentService) Apply(ctx context.Context, input ApplyPrepaymentInput) error {
	pp, err := s.capable("ApplyPrepayment")
	if err != nil {
		return err
	}
	return pp.ApplyPrepayment(ctx, input)
}

// SupportsApplyReceipt reports whether the provider can apply a prepayment
// and return the id of the application it booked (PrepaymentReceiptApplier).
func (s *PrepaymentService) SupportsApplyReceipt() bool {
	_, ok := s.provider.(PrepaymentReceiptApplier)
	return ok
}

// ApplyReceipt is Apply that returns the provider's id for the application.
// ErrUnsupportedProvider when the provider lacks it — never a fallback to
// Apply, whose answer carries no id.
func (s *PrepaymentService) ApplyReceipt(ctx context.Context, input ApplyPrepaymentInput) (*CreatedReceipt, error) {
	ra, ok := s.provider.(PrepaymentReceiptApplier)
	if !ok {
		return nil, &ProviderError{Provider: s.providerName, Op: "ApplyPrepaymentReceipt", Err: ErrUnsupportedProvider}
	}
	return ra.ApplyPrepaymentReceipt(ctx, input)
}

// Unallocate frees a paid amount off an invoice into a new prepayment.
func (s *PrepaymentService) Unallocate(ctx context.Context, input UnallocateToPrepaymentInput) (*Prepayment, error) {
	pp, err := s.capable("UnallocateToPrepayment")
	if err != nil {
		return nil, err
	}
	return pp.UnallocateToPrepayment(ctx, input)
}

// List returns the customer's prepayments with their unapplied remainder.
func (s *PrepaymentService) List(ctx context.Context, input ListPrepaymentsInput) ([]Prepayment, error) {
	pp, err := s.capable("ListPrepayments")
	if err != nil {
		return nil, err
	}
	return pp.ListPrepayments(ctx, input)
}
