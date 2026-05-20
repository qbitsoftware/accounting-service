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

// Get fetches a single customer by its provider-side ID/code. Returns
// ErrNotFound-wrapped error when the customer doesn't exist.
func (s *CustomerService) Get(ctx context.Context, id string) (*Customer, error) {
	return s.provider.GetCustomer(ctx, id)
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

// CustomerListCache lets FindOrCreateWithFallback amortise the broad-search
// List call across many resolutions in a single batch (one List per batch
// rather than one per fallback). Pass nil if you only resolve one customer.
//
// Implementations are expected to be request-scoped — the cache should fetch
// the customer list on first call and return the same slice on subsequent
// calls within the same batch. Concurrency is the implementation's concern;
// the SDK does not lock.
type CustomerListCache interface {
	List(ctx context.Context, client *Client) ([]Customer, error)
}

// FindOrCreateCustomerWithFallback wraps Client.Customers.FindOrCreate with
// a "customer already exists but couldn't be found by email" fallback. Some
// providers (notably Merit) signal IsCustomerExistsError when a customer with
// matching identifiers exists but the email lookup didn't find them — common
// when the stored email differs from the one we're searching with, or when
// providers index strictly. In that case we list all customers and rematch
// by RegNo / email / normalized name (see MatchCustomerFromList).
//
// searchEmail is the email passed to FindCustomerByEmail. input is the full
// CreateCustomerInput used both for create and (via input.Name + input.RegNo)
// for the fallback rematch.
//
// cache may be nil. When non-nil, it provides the customer list for the
// rematch; this saves one List call per resolution in batch operations.
func FindOrCreateCustomerWithFallback(
	ctx context.Context,
	client *Client,
	searchEmail string,
	input CreateCustomerInput,
	cache CustomerListCache,
) (*Customer, error) {
	c, err := client.Customers.FindOrCreate(ctx, searchEmail, input)
	if err == nil {
		return c, nil
	}
	if !IsCustomerExistsError(err) {
		return nil, err
	}

	var customers []Customer
	if cache != nil {
		customers, err = cache.List(ctx, client)
	} else {
		customers, err = client.Customers.List(ctx, ListCustomersInput{})
	}
	if err != nil {
		return nil, err
	}
	return MatchCustomerFromList(customers, input.Name, input.RegNo, searchEmail)
}
