package main

import (
	"context"

	"github.com/my/app/internal/application/profile"
	"github.com/my/app/internal/application/workflows/intake"
	"github.com/my/app/internal/application/workflows/omnichannel"
	reportsmysql "github.com/my/app/internal/repositories/reports/mysql"
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

func (a intakeAssessor) Assess(ctx context.Context, companyID, conversationID int64, sig omnichannel.IntakeSignals) (omnichannel.Completeness, error) {
	res, err := a.svc.Assess(ctx, companyID, conversationID, intake.Signals{
		Sender:          sig.Sender,
		Subject:         sig.Subject,
		Body:            sig.Body,
		ListUnsubscribe: sig.ListUnsubscribe,
		AutoSubmitted:   sig.AutoSubmitted,
		Precedence:      sig.Precedence,
		HasAttachments:  sig.HasAttachments,
		ReferencedCase:  sig.ReferencedCase,
		ThreadMatched:   sig.ThreadMatched,
		Images:          sig.Images,
	})
	if err != nil {
		return omnichannel.Completeness{}, err
	}
	return omnichannel.Completeness{
		Status:         res.Status,
		Missing:        res.Missing,
		Score:          res.Score,
		Reasons:        res.Reasons,
		Classification: res.Classification,
		Reasoning:      res.Reasoning,
		CatalogRelated:   res.CatalogRelated,
		Confidence:       res.Confidence,
		PromoteThreshold: res.PromoteThreshold,
	}, nil
}

type caseLookupAdapter struct {
	repo *reportsmysql.Repository
}

func (a caseLookupAdapter) CaseByCode(ctx context.Context, coverage []int64, code string) (omnichannel.CaseLookupResult, error) {
	row, err := a.repo.CaseByCode(ctx, coverage, code)
	if err != nil {
		return omnichannel.CaseLookupResult{}, err
	}
	return omnichannel.CaseLookupResult{CustomerID: row.CustomerID, SiteID: row.SiteID}, nil
}
