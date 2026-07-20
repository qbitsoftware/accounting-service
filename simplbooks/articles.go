package simplbooks

import (
	"context"
	"fmt"
	"maps"
)

// ListArticles returns items/articles matching the given params (single page).
func (c *Client) ListArticles(ctx context.Context, params ListParams) ([]Article, error) {
	resp, err := c.do(ctx, "GET", "articles/list", params.body())
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Article Article `json:"Article"`
	}
	if err := resp.decodeData(&rows); err != nil {
		return nil, fmt.Errorf("simplbooks: parse articles: %w", err)
	}
	out := make([]Article, len(rows))
	for i := range rows {
		out[i] = rows[i].Article
	}
	return out, nil
}

// ListAllArticles pages through articles/list until exhausted (max 1000/page).
func (c *Client) ListAllArticles(ctx context.Context, filter map[string]any, perPage int) ([]Article, error) {
	if perPage <= 0 || perPage > 1000 {
		perPage = 1000
	}
	var all []Article
	for page := 1; ; page++ {
		batch, err := c.ListArticles(ctx, ListParams{Page: page, PerPage: perPage, Filter: filter})
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < perPage {
			break
		}
	}
	return all, nil
}

// GetArticle returns a single item by ID.
func (c *Client) GetArticle(ctx context.Context, id string) (*Article, error) {
	resp, err := c.do(ctx, "GET", "articles/get/"+id, nil)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Article Article `json:"Article"`
	}
	if err := resp.decodeData(&payload); err != nil {
		return nil, fmt.Errorf("simplbooks: parse article: %w", err)
	}
	return &payload.Article, nil
}

// CreateArticle posts an item and returns the new item ID.
func (c *Client) CreateArticle(ctx context.Context, fields map[string]any) (int, error) {
	resp, err := c.do(ctx, "POST", "articles/create", map[string]any{"Article": fields})
	if err != nil {
		return 0, err
	}
	return resp.insertedID()
}

// UpdateArticle updates an existing item. The ID travels inside the Article body.
func (c *Client) UpdateArticle(ctx context.Context, id int, fields map[string]any) error {
	article := map[string]any{"id": id}
	maps.Copy(article, fields)
	_, err := c.do(ctx, "POST", "articles/update", map[string]any{"Article": article})
	return err
}
