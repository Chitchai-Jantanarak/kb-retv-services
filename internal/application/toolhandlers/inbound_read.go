package toolhandlers

import (
	"context"
	"strconv"

	"github.com/my/app/internal/application/skeleton"
	channelsmysql "github.com/my/app/internal/repositories/channels/mysql"
)

type InboundRepo interface {
	ListInboundDrafts(ctx context.Context, coverage []int64, limit int) ([]channelsmysql.InboundDraftRow, error)
}

type InboundRead struct {
	repo InboundRepo
}

func NewInboundRead(repo InboundRepo) InboundRead {
	return InboundRead{repo: repo}
}

func (h InboundRead) Run(ctx context.Context, q skeleton.Query) ([]skeleton.Row, error) {
	limit := q.Tool.Limit
	if limit <= 0 {
		limit = 10
	}
	drafts, err := h.repo.ListInboundDrafts(ctx, q.Coverage, limit)
	if err != nil {
		return nil, err
	}

	rows := make([]skeleton.Row, 0, len(drafts))
	for _, d := range drafts {
		rows = append(rows, skeleton.Row{
			"subject":  d.Subject,
			"from":     d.From,
			"received": d.Received,
			"id":       strconv.FormatInt(d.ID, 10),
		})
	}
	return rows, nil
}
