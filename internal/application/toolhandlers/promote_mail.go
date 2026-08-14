package toolhandlers

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/shared/perms"
)

type Promoter interface {
	PromoteConversation(ctx context.Context, coverage []int64, conversationID, actorEmployeeID int64) (string, error)
}

var conversationIDRe = regexp.MustCompile(`\b\d+\b`)

type PromoteMail struct {
	repo Promoter
}

func NewPromoteMail(repo Promoter) PromoteMail {
	return PromoteMail{repo: repo}
}

func conversationIDFrom(q skeleton.Query) (int64, bool) {
	raw := q.Params["conversation_id"]
	if raw == "" {
		matches := conversationIDRe.FindAllString(q.Text, 2)
		if len(matches) != 1 {
			return 0, false
		}
		raw = matches[0]
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func (h PromoteMail) Run(ctx context.Context, q skeleton.Query) ([]skeleton.Row, error) {
	if !perms.Can(q.Actor.Perms, "report.create") {
		return nil, fmt.Errorf("promote_mail: not permitted to create cases")
	}
	id, ok := conversationIDFrom(q)
	if !ok {
		return nil, skeleton.NeedsParams("conversation_id")
	}
	code, err := h.repo.PromoteConversation(ctx, q.Coverage, id, q.Actor.UserID)
	if err != nil {
		return nil, err
	}
	return []skeleton.Row{{"code": code}}, nil
}
