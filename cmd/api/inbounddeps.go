package main

import (
	"context"

	"github.com/my/app/internal/application/profile"
	"github.com/my/app/internal/application/workflows/intake"
	"github.com/my/app/internal/application/workflows/omnichannel"
)

type intakeProducts struct {
	repo profile.Repository
}

func (p intakeProducts) Products(ctx context.Context, companyID int64) ([]string, error) {
	data, err := p.repo.Load(ctx, companyID)
	if err != nil {
		return nil, err
	}
	return data.Products, nil
}

type intakeAssessor struct {
	svc *intake.Service
}

func (a intakeAssessor) Assess(ctx context.Context, companyID, conversationID int64, sender, subject, body string) (omnichannel.Completeness, error) {
	res, err := a.svc.Assess(ctx, companyID, conversationID, sender, subject, body)
	if err != nil {
		return omnichannel.Completeness{}, err
	}
	return omnichannel.Completeness{Status: res.Status, Missing: res.Missing, Score: res.Score, Reasons: res.Reasons}, nil
}
