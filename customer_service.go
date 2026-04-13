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
// If the existing customer has a blank or email-fallback name, it updates it.
func (s *CustomerService) FindOrCreate(ctx context.Context, email string, input CreateCustomerInput) (*Customer, error) {
	existing, err := s.provider.FindCustomerByEmail(ctx, email)
	if err != nil && !IsNotFound(err) {
		return nil, err
	}
	if existing != nil {
		// Update name if we now have a real name and the current one looks like a fallback
		if input.Name != "" && input.Name != email && (existing.Name == "" || existing.Name == email || existing.Name == "+") {
			updateInput := UpdateCustomerInput{ID: existing.ID, Name: &input.Name}
			if updateErr := s.provider.UpdateCustomer(ctx, updateInput); updateErr == nil {
				existing.Name = input.Name
			}
		}
		return existing, nil
	}
	return s.provider.CreateCustomer(ctx, input)
}
