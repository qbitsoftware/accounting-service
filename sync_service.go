package accounting

import (
	"context"
	"time"
)

type SyncService struct {
	provider Provider
}

// PullInvoiceStatuses returns invoices changed since the given time, using
// Merit's DateType=1 (changed-date) filter for incremental sync.
func (s *SyncService) PullInvoiceStatuses(ctx context.Context, since time.Time, until time.Time) ([]Invoice, error) {
	return s.provider.ListInvoicesSince(ctx, since, until)
}

// PullPayments returns payments changed since the given time, using
// Merit's DateType=1 (changed-date) filter for incremental sync.
func (s *SyncService) PullPayments(ctx context.Context, since time.Time, until time.Time) ([]Payment, error) {
	return s.provider.ListPaymentsSince(ctx, since, until)
}
