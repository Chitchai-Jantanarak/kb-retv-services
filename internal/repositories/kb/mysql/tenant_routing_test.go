package mysql

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/my/app/internal/application/services/kbbootstrap"
	"github.com/my/app/internal/shared/ctxkey"
)

var errCapture = errors.New("captured")

type capturingQuerier struct {
	companyID int64
	hit       bool
}

func (c *capturingQuerier) capture(ctx context.Context) {
	if cid, ok := ctxkey.CompanyID(ctx); ok {
		c.companyID = cid
		c.hit = true
	}
}

func (c *capturingQuerier) QueryContext(ctx context.Context, _ string, _ ...any) (*sql.Rows, error) {
	c.capture(ctx)
	return nil, errCapture
}

func (c *capturingQuerier) QueryRowContext(ctx context.Context, _ string, _ ...any) *sql.Row {
	c.capture(ctx)
	return nil
}

func (c *capturingQuerier) ExecContext(ctx context.Context, _ string, _ ...any) (sql.Result, error) {
	c.capture(ctx)
	return nil, errCapture
}

func (c *capturingQuerier) BeginTx(ctx context.Context, _ *sql.TxOptions) (*sql.Tx, error) {
	c.capture(ctx)
	return nil, errCapture
}

func TestIngestRepositoriesRouteByCompanyID(t *testing.T) {
	t.Run("report_source", func(t *testing.T) {
		q := &capturingQuerier{}
		_, _ = NewReportSource(q).Reports(context.Background(), 3, 0)
		assertRouted(t, q)
	})

	t.Run("article_store", func(t *testing.T) {
		q := &capturingQuerier{}
		_, _ = NewArticleStore(q).InsertArticle(context.Background(),
			kbbootstrap.ArticleRecord{SourceReportID: 1, CompanyID: 3}, []string{"chunk"})
		assertRouted(t, q)
	})

	t.Run("article_source", func(t *testing.T) {
		q := &capturingQuerier{}
		_, _ = NewArticleSource(q).ArticlesSince(context.Background(), 3, time.Time{}, 10)
		assertRouted(t, q)
	})

	t.Run("chunk_source", func(t *testing.T) {
		q := &capturingQuerier{}
		_, _ = NewChunkSourceForModel(q, "model").Next(context.Background(), 3, 0, 10)
		assertRouted(t, q)
	})
}

func assertRouted(t *testing.T, q *capturingQuerier) {
	t.Helper()
	if !q.hit {
		t.Fatal("querier was never reached")
	}
	if q.companyID != 3 {
		t.Fatalf("company_id in context = %d, want 3 (tenant router would target the wrong DB)", q.companyID)
	}
}
