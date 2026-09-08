package tools

import (
	"context"
	"errors"
	"testing"
)

type fakeToolSelector struct {
	sel Selection
	err error
}

func (f *fakeToolSelector) Select(ctx context.Context, text string, perms []string) (Selection, error) {
	return f.sel, f.err
}

func TestCascadeTakesTier0WhenBarCleared(t *testing.T) {
	first := &fakeToolSelector{sel: Selection{ToolID: "f1", Matched: true, Score: 0.9, RunnerUp: 0.1}}
	second := &fakeToolSelector{err: errors.New("tier 2 should not be called")}
	c := NewCascade(first, second, 0.75, 0.15)

	got, err := c.Select(context.Background(), "find my case", nil)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got.ToolID != "f1" {
		t.Fatalf("Select() = %+v, want tier-0 result f1", got)
	}
}

func TestCascadeFallsThroughOnLowScore(t *testing.T) {
	first := &fakeToolSelector{sel: Selection{ToolID: "f1", Matched: true, Score: 0.6, RunnerUp: 0.1}}
	second := &fakeToolSelector{sel: Selection{ToolID: "f2", Matched: true}}
	c := NewCascade(first, second, 0.75, 0.15)

	got, err := c.Select(context.Background(), "ambiguous message", nil)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got.ToolID != "f2" {
		t.Fatalf("Select() = %+v, want tier-2 result f2", got)
	}
}

func TestCascadeFallsThroughOnNarrowMargin(t *testing.T) {
	first := &fakeToolSelector{sel: Selection{ToolID: "f1", Matched: true, Score: 0.9, RunnerUp: 0.8}}
	second := &fakeToolSelector{sel: Selection{ToolID: "f2", Matched: true}}
	c := NewCascade(first, second, 0.75, 0.15)

	got, err := c.Select(context.Background(), "ambiguous message", nil)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got.ToolID != "f2" {
		t.Fatalf("Select() = %+v, want tier-2 result f2 on narrow margin", got)
	}
}

func TestCascadeDegradesToTier0WhenTier2Errors(t *testing.T) {
	tier0Err := errors.New("tier 0 embed failure")
	first := &fakeToolSelector{sel: Selection{ToolID: "f1"}, err: tier0Err}
	second := &fakeToolSelector{err: errors.New("tier 2 down")}
	c := NewCascade(first, second, 0.75, 0.15)

	got, err := c.Select(context.Background(), "small talk", nil)
	if !errors.Is(err, tier0Err) {
		t.Fatalf("Select() error = %v, want tier-0 error %v surfaced when tier 2 also fails", err, tier0Err)
	}
	if got.ToolID != "f1" {
		t.Fatalf("Select() = %+v, want tier-0 result on tier-2 failure", got)
	}
}
