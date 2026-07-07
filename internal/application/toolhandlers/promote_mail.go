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

func (h PromoteMail) Run(ctx context.Context, q skeleton.Query) ([]skeleton.Row, error) {
	if !perms.Can(q.Actor.Perms, "report.create") {
		return nil, fmt.Errorf("promote_mail: not permitted to create cases")
	}
	match := conversationIDRe.FindString(q.Text)
	if match == "" {
		return nil, fmt.Errorf("promote_mail: specify which email (id)")
	}
	id, err := strconv.ParseInt(match, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("promote_mail: specify which email (id)")
	}
	code, err := h.repo.PromoteConversation(ctx, q.Coverage, id, q.Actor.UserID)
	if err != nil {
		return nil, err
	}
	return []skeleton.Row{{"code": code}}, nil
}
