package rag

import (
	"strings"
	"time"
)

func timed(timings map[string]int64, name string, fn func() error) error {
	if timings == nil {
		return fn()
	}
	start := time.Now()
	err := fn()
	timings[name] = time.Since(start).Milliseconds()
	return err
}

func newStageTimings(query Query) map[string]int64 {
	if !wantsTimings(query) {
		return nil
	}
	return make(map[string]int64)
}

func wantsTimings(query Query) bool {
	return query.Debug || query.Mode == ModeDebug
}

func debugTrace(
	query Query,
	plan QueryPlan,
	candidates []Candidate,
	contextAssembled bool,
	crag CRAGResult,
	confidence float64,
	threshold float64,
	decision Decision,
	draft string,
	criticRan bool,
	graphError string,
) *DebugTrace {
	if !wantsTimings(query) {
		return nil
	}
	topSource := ""
	if len(candidates) > 0 {
		topSource = candidates[0].Source
	}
	return &DebugTrace{
		Mode: query.Mode,
		Plan: DebugPlanTrace{
			Terms:            append([]string(nil), plan.Meta.Terms...),
			RetrievalQueries: len(retrievalQueriesForPlan(query, plan)),
			GraphAnchors:     len(plan.GraphAnchors),
			SkipReasons:      copyStringMap(plan.SkipReasons),
		},
		Retrieval: DebugRetrievalTrace{
			Candidates:       len(candidates),
			TopSource:        topSource,
			ContextAssembled: contextAssembled,
			GraphError:       graphError,
		},
		Quality: DebugQualityTrace{
			CRAGVerdict:    crag.Verdict.String(),
			CRAGConfidence: crag.Confidence,
			CRAGMissing:    crag.Missing,
			Confidence:     confidence,
			Threshold:      threshold,
			Decision:       string(decision),
			DraftGenerated: strings.TrimSpace(draft) != "",
			CriticRan:      criticRan,
		},
	}
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
