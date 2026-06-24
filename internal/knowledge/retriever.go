package knowledge

import (
	"context"
	"strings"

	"github.com/Floodnut/ergo-loom/internal/core"
	"github.com/Floodnut/ergo-loom/internal/storage/sqlitecli"
)

type SearchStore interface {
	SearchKnowledge(options sqlitecli.KnowledgeSearchOptions) ([]core.KnowledgeItem, error)
}

type KeywordRetriever struct {
	store SearchStore
}

func NewKeywordRetriever(store SearchStore) KeywordRetriever {
	return KeywordRetriever{store: store}
}

func (r KeywordRetriever) Search(ctx context.Context, q core.KnowledgeQuery) ([]core.KnowledgeItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	query := strings.TrimSpace(q.Text)
	if q.Scope != "" {
		return r.store.SearchKnowledge(sqlitecli.KnowledgeSearchOptions{
			Query:     query,
			Scope:     q.Scope,
			ProjectID: q.ProjectID,
			Limit:     limit,
		})
	}
	if strings.TrimSpace(q.ProjectID) == "" {
		return r.store.SearchKnowledge(sqlitecli.KnowledgeSearchOptions{
			Query: query,
			Scope: core.KnowledgeScopeGlobal,
			Limit: limit,
		})
	}

	projectItems, err := r.store.SearchKnowledge(sqlitecli.KnowledgeSearchOptions{
		Query:     query,
		Scope:     core.KnowledgeScopeProject,
		ProjectID: q.ProjectID,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	globalItems, err := r.store.SearchKnowledge(sqlitecli.KnowledgeSearchOptions{
		Query: query,
		Scope: core.KnowledgeScopeGlobal,
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	return firstUnique(append(projectItems, globalItems...), limit), nil
}

func firstUnique(items []core.KnowledgeItem, limit int) []core.KnowledgeItem {
	if limit <= 0 {
		limit = len(items)
	}
	out := make([]core.KnowledgeItem, 0, min(limit, len(items)))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if item.ID == "" || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out
}
