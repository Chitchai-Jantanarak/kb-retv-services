package mysql

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/my/app/internal/domain/kb"
	"github.com/my/app/internal/domain/retrieval"
)

func TestKnowledgeRepositorySearchIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := integrationDB(t)
	seed := seedKnowledgeArticle(t, ctx, db)

	repo := NewKnowledgeRepository(db)
	articles, err := repo.Search(ctx, retrieval.Query{
		CompanyID: seed.companyID,
		Text:      seed.title,
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(articles) != 1 {
		t.Fatalf("len(articles) = %d, want 1", len(articles))
	}
	if articles[0].CompanyID != seed.companyID {
		t.Fatalf("CompanyID = %d, want %d", articles[0].CompanyID, seed.companyID)
	}
	if articles[0].ID != idToString(seed.articleID) {
		t.Fatalf("ID = %q, want %d", articles[0].ID, seed.articleID)
	}
}

func TestKnowledgeRepositoryParentChunksByChildIDsIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := integrationDB(t)
	seed := seedKnowledgeArticle(t, ctx, db)

	repo := NewKnowledgeRepository(db)
	contexts, err := repo.ParentChunksByChildIDs(ctx, kb.ChunkQuery{
		CompanyID: seed.companyID,
		IDs:       []string{idToString(seed.childChunkID)},
	})
	if err != nil {
		t.Fatalf("ParentChunksByChildIDs() error = %v", err)
	}
	if len(contexts) != 1 {
		t.Fatalf("len(contexts) = %d, want 1", len(contexts))
	}
	if contexts[0].ChildID != idToString(seed.childChunkID) {
		t.Fatalf("ChildID = %q, want %d", contexts[0].ChildID, seed.childChunkID)
	}
	if contexts[0].ParentID != idToString(seed.parentChunkID) {
		t.Fatalf("ParentID = %q, want %d", contexts[0].ParentID, seed.parentChunkID)
	}
	if contexts[0].Content != seed.parentContent {
		t.Fatalf("Content = %q, want parent content", contexts[0].Content)
	}
}

type knowledgeSeed struct {
	companyID     int64
	articleID     int64
	parentChunkID int64
	childChunkID  int64
	title         string
	parentContent string
}

func integrationDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN is not set")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedKnowledgeArticle(t *testing.T, ctx context.Context, db *sql.DB) knowledgeSeed {
	t.Helper()

	seed := knowledgeSeed{
		companyID:     3,
		title:         "RAG integration temporary article " + time.Now().Format("20060102150405.000000000"),
		parentContent: "Parent context: reconnect network, check packaging, verify order.",
	}

	res, err := db.ExecContext(ctx, `
INSERT INTO kb_articles (company_id, title, body, visibility, status, lang, source)
VALUES (?, ?, ?, 'internal', 'published', 'en', 'integration_test')`,
		seed.companyID, seed.title, seed.parentContent)
	if err != nil {
		t.Fatalf("insert kb_article: %v", err)
	}
	seed.articleID, err = res.LastInsertId()
	if err != nil {
		t.Fatalf("article LastInsertId: %v", err)
	}

	res, err = db.ExecContext(ctx, `
INSERT INTO kb_chunks (kb_article_id, chunk_index, content, token_count, chunk_kind)
VALUES (?, -1, ?, ?, 'parent')`,
		seed.articleID, seed.parentContent, len([]rune(seed.parentContent)))
	if err != nil {
		t.Fatalf("insert parent chunk: %v", err)
	}
	seed.parentChunkID, err = res.LastInsertId()
	if err != nil {
		t.Fatalf("parent LastInsertId: %v", err)
	}

	childContent := "Package damaged after restart, order sync failed."
	res, err = db.ExecContext(ctx, `
INSERT INTO kb_chunks (kb_article_id, chunk_index, content, token_count, chunk_kind, parent_chunk_id)
VALUES (?, 0, ?, ?, 'child', ?)`,
		seed.articleID, childContent, len([]rune(childContent)), seed.parentChunkID)
	if err != nil {
		t.Fatalf("insert child chunk: %v", err)
	}
	seed.childChunkID, err = res.LastInsertId()
	if err != nil {
		t.Fatalf("child LastInsertId: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM kb_embeddings WHERE kb_chunk_id IN (?, ?)`, seed.parentChunkID, seed.childChunkID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM kb_chunks WHERE id IN (?, ?)`, seed.childChunkID, seed.parentChunkID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM kb_articles WHERE id = ?`, seed.articleID)
	})

	return seed
}
