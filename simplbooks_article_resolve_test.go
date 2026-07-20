package accounting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestResolveArticleID covers how an invoice line's Code becomes a SimplBooks
// article_id — the mapping that lets klubio keep readable codes ("Liikmemaks")
// instead of opaque numeric ids.
func TestResolveArticleID(t *testing.T) {
	var listCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		listCalls++
		json.NewEncoder(w).Encode(map[string]any{
			"status": 200,
			"data": []any{
				map[string]any{"Article": map[string]any{"id": 7, "code": "Liikmemaks", "name": "Liikmemaks"}},
				map[string]any{"Article": map[string]any{"id": 9, "code": "TARISTU", "name": "Taristumaks"}},
			},
		})
	}))
	defer srv.Close()

	p := newSimplBooksProvider(Config{
		APIID: "co", APIKey: "tok",
		Extra: map[string]string{"base_url": srv.URL},
	})
	ctx := context.Background()

	cases := []struct {
		name string
		code string
		want int
	}{
		{"exact code match", "Liikmemaks", 7},
		{"case-insensitive", "liikmemaks", 7},
		{"surrounding whitespace", "  TARISTU  ", 9},
		{"numeric code matching a real article id", "7", 7},
		// The existence check is the point: without it a klubio code like "1001"
		// would link to whatever unrelated article held id 1001.
		{"numeric code with no such article id", "1001", 0},
		{"unknown code falls back to free text", "NOPE", 0},
		{"empty code", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := p.resolveArticleID(ctx, c.code); got != c.want {
				t.Errorf("resolveArticleID(%q) = %d, want %d", c.code, got, c.want)
			}
		})
	}
}

// The register is fetched once and reused: klubio builds one client per sync
// run, so a batch must not spend its 60 req/min budget re-listing articles.
func TestResolveArticleIDCachesTheRegister(t *testing.T) {
	var listCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		listCalls++
		json.NewEncoder(w).Encode(map[string]any{
			"status": 200,
			"data": []any{
				map[string]any{"Article": map[string]any{"id": 7, "code": "Liikmemaks"}},
			},
		})
	}))
	defer srv.Close()

	p := newSimplBooksProvider(Config{APIID: "co", APIKey: "tok", Extra: map[string]string{"base_url": srv.URL}})
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if got := p.resolveArticleID(ctx, "Liikmemaks"); got != 7 {
			t.Fatalf("resolve #%d = %d, want 7", i, got)
		}
	}
	if listCalls != 1 {
		t.Errorf("articles/list called %d times, want 1 (cache not reused)", listCalls)
	}

	// A miss refreshes once, in case the article was created mid-run.
	before := listCalls
	p.resolveArticleID(ctx, "BRAND-NEW")
	if listCalls != before+1 {
		t.Errorf("a miss should refresh exactly once: calls went %d → %d", before, listCalls)
	}

	// After an explicit invalidation the next resolve reloads.
	p.invalidateArticleCache()
	before = listCalls
	p.resolveArticleID(ctx, "Liikmemaks")
	if listCalls != before+1 {
		t.Errorf("invalidation should force a reload: calls went %d → %d", before, listCalls)
	}
}
