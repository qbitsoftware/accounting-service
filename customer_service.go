package accounting

import "context"

type CustomerService struct {
	provider Provider
}

func (s *CustomerService) Create(ctx context.Context, input CreateCustomerInput) (*Customer, error) {
	return s.provider.CreateCustomer(ctx, input)
}

func (s *CustomerService) Update(ctx context.Context, input UpdateCustomerInput) error {
	return s.provider.UpdateCustomer(ctx, input)
}

func (s *CustomerService) List(ctx context.Context, input ListCustomersInput) ([]Customer, error) {
	return s.provider.ListCustomers(ctx, input)
}

// FindOrCreate searches for a customer by email and returns it if found,
// otherwise creates a new customer with the provided input.
func (s *CustomerService) FindOrCreate(ctx context.Context, email string, input CreateCustomerInput) (*Customer, error) {
	existing, err := s.provider.FindCustomerByEmail(ctx, email)
	if err != nil && !IsNotFound(err) {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	return s.provider.CreateCustomer(ctx, input)
}
