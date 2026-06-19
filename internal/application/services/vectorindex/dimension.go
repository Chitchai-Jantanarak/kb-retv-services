package vectorindex

import (
	"context"
	"errors"
	"fmt"
)

type CollectionManager interface {
	CollectionVectorSize(ctx context.Context, name string) (int, bool, error)
	EnsureCollection(ctx context.Context, name string, dim int) error
	RecreateCollection(ctx context.Context, name string, dim int) error
}

type DimensionAction int

const (
	DimensionUnchanged DimensionAction = iota
	DimensionCreated
	DimensionRecreated
)

func (a DimensionAction) String() string {
	switch a {
	case DimensionCreated:
		return "created"
	case DimensionRecreated:
		return "recreated"
	default:
		return "unchanged"
	}
}

func decideDimensionAction(currentDim int, exists bool, embedDim int) (DimensionAction, error) {
	if embedDim <= 0 {
		return DimensionUnchanged, errors.New("embedder dimension must be positive")
	}
	switch {
	case !exists:
		return DimensionCreated, nil
	case currentDim != embedDim:
		return DimensionRecreated, nil
	default:
		return DimensionUnchanged, nil
	}
}

func ReconcileDimension(ctx context.Context, mgr CollectionManager, collection string, embedDim int) (DimensionAction, error) {
	currentDim, exists, err := mgr.CollectionVectorSize(ctx, collection)
	if err != nil {
		return DimensionUnchanged, fmt.Errorf("inspect collection %s: %w", collection, err)
	}

	action, err := decideDimensionAction(currentDim, exists, embedDim)
	if err != nil {
		return DimensionUnchanged, err
	}

	switch action {
	case DimensionCreated:
		if err := mgr.EnsureCollection(ctx, collection, embedDim); err != nil {
			return DimensionUnchanged, fmt.Errorf("create collection %s: %w", collection, err)
		}
	case DimensionRecreated:
		if err := mgr.RecreateCollection(ctx, collection, embedDim); err != nil {
			return DimensionUnchanged, fmt.Errorf("recreate collection %s: %w", collection, err)
		}
	}
	return action, nil
}
