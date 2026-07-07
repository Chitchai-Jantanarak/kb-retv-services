package toolhandlers

import (
	"context"

	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/shared/perms"
)

type OwnershipRepo interface {
	IsResponsible(ctx context.Context, companyID, reportID, userID int64) (bool, error)
}

func CanMutate(ctx context.Context, repo OwnershipRepo, actor skeleton.Actor, reportID int64) bool {
	if perms.Can(actor.Perms, "report.assign") {
		return true
	}
	ok, err := repo.IsResponsible(ctx, actor.CompanyID, reportID, actor.UserID)
	return err == nil && ok
}
