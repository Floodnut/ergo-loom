package knowledge

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Floodnut/ergo-loom/internal/core"
	"github.com/Floodnut/ergo-loom/internal/storage/sqlitecli"
)

func TestKeywordRetrieverReturnsProjectAndGlobalKnowledge(t *testing.T) {
	dir := t.TempDir()
	store := sqlitecli.Store{
		DBPath:     filepath.Join(dir, "ergo.db"),
		SchemaPath: filepath.Join("..", "storage", "sqlitecli", "schema.sql"),
	}
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	project, err := store.CreateProject("KB Project", "/tmp/kb-project")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	projectItem, err := store.AddKnowledgeItem(core.KnowledgeItem{
		Scope:     core.KnowledgeScopeProject,
		ProjectID: project.ID,
		Kind:      "note",
		Title:     "OpenStack troubleshooting",
	})
	if err != nil {
		t.Fatalf("add project knowledge: %v", err)
	}
	globalItem, err := store.AddKnowledgeItem(core.KnowledgeItem{
		Scope: core.KnowledgeScopeGlobal,
		Kind:  "note",
		Title: "OpenStack quota convention",
	})
	if err != nil {
		t.Fatalf("add global knowledge: %v", err)
	}

	items, err := NewKeywordRetriever(store).Search(context.Background(), core.KnowledgeQuery{
		ProjectID: project.ID,
		Text:      "OpenStack",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("search knowledge: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected project and global items, got %#v", items)
	}
	if items[0].ID != projectItem.ID || items[1].ID != globalItem.ID {
		t.Fatalf("unexpected search order: %#v", items)
	}
}
