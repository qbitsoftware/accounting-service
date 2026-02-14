package accounting

import "context"

type ItemService struct {
	provider Provider
}

func (s *ItemService) Create(ctx context.Context, input CreateItemInput) (*Item, error) {
	return s.provider.CreateItem(ctx, input)
}

func (s *ItemService) List(ctx context.Context, input ListItemsInput) ([]Item, error) {
	return s.provider.ListItems(ctx, input)
}

func (s *ItemService) Update(ctx context.Context, input UpdateItemInput) error {
	return s.provider.UpdateItem(ctx, input)
}
