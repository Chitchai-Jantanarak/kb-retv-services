package rag

import "context"

type CRAGVerdict int

const (
	VerdictRelevant CRAGVerdict = iota
	VerdictPartial
	VerdictIrrelevant
)

type CRAGResult struct {
	Verdict    CRAGVerdict
	Confidence float64
	Missing    string
	Graded     bool
}

func (v CRAGVerdict) String() string {
	switch v {
	case VerdictRelevant:
		return "relevant"
	case VerdictPartial:
		return "partial"
	case VerdictIrrelevant:
		return "irrelevant"
	default:
		return "unknown"
	}
}

type CRAG interface {
	Grade(ctx context.Context, query string, content string) (CRAGResult, error)
}

type PassthroughCRAG struct{}

func (PassthroughCRAG) Grade(_ context.Context, _, _ string) (CRAGResult, error) {
	return CRAGResult{Verdict: VerdictRelevant}, nil
}
