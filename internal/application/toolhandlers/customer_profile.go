package toolhandlers

import (
	"context"
	"strings"

	"github.com/my/app/internal/application/skeleton"
	customermysql "github.com/my/app/internal/repositories/customer/mysql"
)

type CustomerRepo interface {
	ProfileByName(ctx context.Context, coverage []int64, name string) (customermysql.Profile, error)
}

type CustomerProfile struct {
	repo CustomerRepo
}

func NewCustomerProfile(repo CustomerRepo) CustomerProfile {
	return CustomerProfile{repo: repo}
}

func (h CustomerProfile) Run(ctx context.Context, q skeleton.Query) ([]skeleton.Row, error) {
	customer := q.Params["customer"]
	if customer == "" {
		customer = strings.TrimSpace(q.Text)
	}
	profile, err := h.repo.ProfileByName(ctx, q.Coverage, customer)
	if err != nil {
		return nil, err
	}
	if !profile.Found {
		return []skeleton.Row{}, nil
	}
	rows := make([]skeleton.Row, 0, 4)
	for _, section := range []struct{ name, detail string }{
		{"company", profile.Company},
		{"status", profile.Status},
		{"address", profile.Address},
		{"website", profile.Website},
	} {
		if section.detail == "" {
			continue
		}
		rows = append(rows, skeleton.Row{"section": section.name, "detail": section.detail})
	}
	return rows, nil
}
