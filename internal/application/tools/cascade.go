package tools

import "context"

type ToolSelectorLike interface {
	Select(ctx context.Context, text string, perms []string) (Selection, error)
}

type Cascade struct {
	first  ToolSelectorLike
	second ToolSelectorLike
	accept float64
	margin float64
}

func NewCascade(first, second ToolSelectorLike, accept, margin float64) *Cascade {
	return &Cascade{first: first, second: second, accept: accept, margin: margin}
}

func (c *Cascade) Select(ctx context.Context, text string, perms []string) (Selection, error) {
	s, err := c.first.Select(ctx, text, perms)
	if err == nil && s.Matched && s.Score >= c.accept && (s.Score-s.RunnerUp) >= c.margin {
		return s, nil
	}

	s2, err2 := c.second.Select(ctx, text, perms)
	if err2 == nil {
		return s2, nil
	}

	return s, err
}
