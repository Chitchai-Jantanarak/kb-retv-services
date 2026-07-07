package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type InboundDraftRow struct {
	ID       int64
	Subject  string
	From     string
	Received string
}

func buildListInboundDraftsQuery(coverage []int64, limit int) (string, []any) {
	if len(coverage) == 0 {
		return "", nil
	}
	if limit <= 0 {
		limit = 10
	}
	placeholders := make([]string, len(coverage))
	args := make([]any, 0, len(coverage)+1)
	for i, id := range coverage {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, limit)

	query := fmt.Sprintf(`
SELECT id, COALESCE(subject, ''), COALESCE(external_customer, ''), COALESCE(last_message_at, '')
FROM conversations
WHERE company_id IN (%s) AND status = 'pending'
ORDER BY last_message_at DESC
LIMIT ?`, strings.Join(placeholders, ","))
	return query, args
}

func (r *Repository) ListInboundDrafts(ctx context.Context, coverage []int64, limit int) ([]InboundDraftRow, error) {
	if len(coverage) == 0 {
		return []InboundDraftRow{}, nil
	}
	query, args := buildListInboundDraftsQuery(coverage, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("conversations: list inbound drafts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]InboundDraftRow, 0)
	for rows.Next() {
		var id int64
		var subject, from, received sql.NullString
		if err := rows.Scan(&id, &subject, &from, &received); err != nil {
			return nil, fmt.Errorf("conversations: scan inbound draft: %w", err)
		}
		out = append(out, InboundDraftRow{ID: id, Subject: subject.String, From: from.String, Received: received.String})
	}
	return out, rows.Err()
}
