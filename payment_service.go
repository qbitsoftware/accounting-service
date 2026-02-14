package accounting

import "context"

type PaymentService struct {
	provider Provider
}

func (s *PaymentService) Create(ctx context.Context, input CreatePaymentInput) error {
	return s.provider.CreatePayment(ctx, input)
}

func (s *PaymentService) List(ctx context.Context, input ListPaymentsInput) ([]Payment, error) {
	return s.provider.ListPayments(ctx, input)
}

func (s *PaymentService) Delete(ctx context.Context, id string) error {
	return s.provider.DeletePayment(ctx, id)
}
