package accounting

import "context"

type ReportService struct {
	provider Provider
}

func (s *ReportService) CustomerDebts(ctx context.Context, customerName string, overdueDays *int) ([]CustomerDebt, error) {
	return s.provider.CustomerDebts(ctx, customerName, overdueDays)
}
