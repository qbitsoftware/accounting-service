package smartaccounts

import (
	"context"
	"net/url"
)

// ListArticlesParams filters an articles:get query.
type ListArticlesParams struct {
	SearchString string
	Code         string
	ModifiedFrom string // dd.MM.yyyy
	ModifiedTo   string // dd.MM.yyyy
}

func (p ListArticlesParams) values() url.Values {
	v := url.Values{}
	setNonEmpty(v, "searchString", p.SearchString)
	setNonEmpty(v, "code", p.Code)
	setNonEmpty(v, "modifiedFrom", p.ModifiedFrom)
	setNonEmpty(v, "modifiedTo", p.ModifiedTo)
	return v
}

// ListArticles retrieves articles matching params, following pagination.
func (c *Client) ListArticles(ctx context.Context, params ListArticlesParams) ([]ArticleItem, error) {
	var items []ArticleItem
	if _, err := c.getList(ctx, "purchasesales/articles:get", params.values(), &items); err != nil {
		return nil, err
	}
	return items, nil
}

// CreateArticle adds an article.
func (c *Client) CreateArticle(ctx context.Context, req ArticleItem) (*ArticleResponse, error) {
	var resp ArticleResponse
	if err := c.post(ctx, "purchasesales/articles:add", nil, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// EditArticle updates an article (req.Code identifies it).
func (c *Client) EditArticle(ctx context.Context, req ArticleItem) error {
	return c.post(ctx, "purchasesales/articles:edit", nil, req, nil)
}
