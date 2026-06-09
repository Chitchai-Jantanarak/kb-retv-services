// Package promote applies a review_queue decision to the canonical
// tables. Same shape as omnichannel: one Workflow type, kind-keyed
// dispatch, per-kind helpers.
package promote

import (
	"context"
	"errors"
	"fmt"
)

type Kind string

const (
	KindKB      Kind = "kb_promotion"
	KindSymptom Kind = "symptom_proposed"
	KindSubject Kind = "subject_proposed"
)

type ReviewItem struct {
	ID        int64
	CompanyID int64
	Kind      Kind
	Payload   map[string]any
}

type Repo interface {
	Get(ctx context.Context, reviewItemID int64) (ReviewItem, error)
	MarkApproved(ctx context.Context, reviewItemID int64, promotionRef string) error
	ActivateSymptom(ctx context.Context, companyID, symptomNodeID int64) error
	ActivateSubject(ctx context.Context, companyID, subjectNodeID int64) error
}

type Workflow struct {
	repo Repo
}

func New(repo Repo) (*Workflow, error) {
	if repo == nil {
		return nil, errors.New("promote: repo is required")
	}
	return &Workflow{repo: repo}, nil
}

type Result struct {
	ReviewItemID int64
	Kind         Kind
	PromotionRef string
}

func (w *Workflow) Run(ctx context.Context, reviewItemID int64) (Result, error) {
	return w.runFor(ctx, 0, reviewItemID)
}

func (w *Workflow) RunFor(ctx context.Context, companyID, reviewItemID int64) (Result, error) {
	if companyID <= 0 {
		return Result{}, errors.New("promote: company id must be positive")
	}
	return w.runFor(ctx, companyID, reviewItemID)
}

func (w *Workflow) runFor(ctx context.Context, expectCompanyID, reviewItemID int64) (Result, error) {
	if reviewItemID <= 0 {
		return Result{}, errors.New("promote: review item id must be positive")
	}
	item, err := w.repo.Get(ctx, reviewItemID)
	if err != nil {
		return Result{}, fmt.Errorf("promote: load item %d: %w", reviewItemID, err)
	}
	if item.CompanyID <= 0 {
		return Result{}, fmt.Errorf("promote: item %d has no company_id", reviewItemID)
	}
	if expectCompanyID > 0 && item.CompanyID != expectCompanyID {
		return Result{}, fmt.Errorf("promote: item %d belongs to company %d, expected %d", reviewItemID, item.CompanyID, expectCompanyID)
	}

	ref, err := w.dispatch(ctx, item)
	if err != nil {
		return Result{}, err
	}
	if err := w.repo.MarkApproved(ctx, item.ID, ref); err != nil {
		return Result{}, fmt.Errorf("promote: mark approved: %w", err)
	}
	return Result{ReviewItemID: item.ID, Kind: item.Kind, PromotionRef: ref}, nil
}

func (w *Workflow) dispatch(ctx context.Context, item ReviewItem) (string, error) {
	switch item.Kind {
	case KindSymptom:
		id, err := requireInt64(item.Payload, "symptom_node_id")
		if err != nil {
			return "", err
		}
		if err := w.repo.ActivateSymptom(ctx, item.CompanyID, id); err != nil {
			return "", fmt.Errorf("promote: activate symptom %d: %w", id, err)
		}
		return fmt.Sprintf("symptom:%d", id), nil
	case KindSubject:
		id, err := requireInt64(item.Payload, "subject_node_id")
		if err != nil {
			return "", err
		}
		if err := w.repo.ActivateSubject(ctx, item.CompanyID, id); err != nil {
			return "", fmt.Errorf("promote: activate subject %d: %w", id, err)
		}
		return fmt.Sprintf("subject:%d", id), nil
	case KindKB:
		id, err := requireInt64(item.Payload, "kb_article_id")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("kb_article:%d", id), nil
	default:
		return "", fmt.Errorf("promote: unsupported kind %q", item.Kind)
	}
}

func requireInt64(p map[string]any, key string) (int64, error) {
	v, ok := p[key]
	if !ok {
		return 0, fmt.Errorf("promote: payload missing %q", key)
	}
	switch n := v.(type) {
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	case float64:
		return int64(n), nil
	default:
		return 0, fmt.Errorf("promote: payload %q has unsupported type %T", key, v)
	}
}
