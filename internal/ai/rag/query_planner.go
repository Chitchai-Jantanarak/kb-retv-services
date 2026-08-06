package rag

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

const (
	defaultQueryPlanLimit  = 5
	maxQueryPlanLimit      = 10
	minAISymptomConfidence = 0.70
)

type DefaultQueryPlanner struct{}

func (p DefaultQueryPlanner) Plan(ctx context.Context, query Query) (QueryPlan, error) {
	if err := ctx.Err(); err != nil {
		return QueryPlan{}, err
	}

	retrievalText := collapseWhitespace(query.Text)
	if retrievalText == "" {
		return QueryPlan{}, errors.New("query text is required")
	}

	plannedQuery := query
	plannedQuery.Text = retrievalText
	plannedQuery.Limit = effectiveQueryLimit(query.Limit)

	meta := Meta{Terms: terms(plannedQuery.Text)}

	canonicalText := canonicalQueryText(retrievalText)
	navigational := isNavigationalQuery(canonicalText)
	graphAnchors, graphSkipReason := graphAnchorsFromMetadata(query.Metadata, navigational)
	return QueryPlan{
		OriginalText:   query.Text,
		RetrievalText:  retrievalText,
		CanonicalText:  canonicalText,
		Meta:           meta,
		EffectiveLimit: plannedQuery.Limit,
		IsNavigational: navigational,
		RetrievalQueries: []PlannedQuery{
			{Kind: PlannedQueryOriginal, Text: retrievalText},
		},
		GraphAnchors: graphAnchors,
		SkipReasons:  plannerSkipReasons(graphSkipReason),
	}, nil
}

func effectiveQueryLimit(limit int) int {
	switch {
	case limit <= 0:
		return defaultQueryPlanLimit
	case limit > maxQueryPlanLimit:
		return maxQueryPlanLimit
	default:
		return limit
	}
}

func collapseWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func canonicalQueryText(text string) string {
	return strings.ToLower(collapseWhitespace(text))
}

func isNavigationalQuery(canonical string) bool {
	for _, prefix := range []string{
		"open ",
		"go to ",
		"navigate to ",
		"take me to ",
		"where is ",
		"link to ",
	} {
		if strings.HasPrefix(canonical, prefix) {
			return true
		}
	}
	return false
}

func plannerSkipReasons(graphSkipReason string) map[string]string {
	if graphSkipReason == "" {
		return nil
	}
	return map[string]string{"graph_anchor": graphSkipReason}
}

func graphAnchorsFromMetadata(metadata map[string]string, navigational bool) ([]GraphAnchor, string) {
	if navigational {
		return nil, "disabled for navigational query"
	}
	if len(metadata) == 0 {
		return nil, "no symptom metadata"
	}

	normalized := normalizePlannerMetadata(metadata)
	if id, ok := metadataInt64(normalized, "symptom_node_id", "symptom_id"); ok {
		return []GraphAnchor{{Kind: GraphAnchorSymptom, ID: id}}, ""
	}

	aiID, hasAIID := metadataInt64(normalized, "ai_symptom_node_id", "ai_symptom_id")
	if !hasAIID {
		return nil, "no symptom metadata"
	}
	conf, hasConf := metadataFloat64(normalized, "ai_symptom_conf", "ai_symptom_confidence", "symptom_conf", "symptom_confidence")
	if !hasConf {
		return nil, "ai symptom confidence missing"
	}
	if conf < minAISymptomConfidence {
		return nil, "ai symptom confidence below graph anchor threshold"
	}
	return []GraphAnchor{{Kind: GraphAnchorSymptom, ID: aiID}}, ""
}

func normalizePlannerMetadata(metadata map[string]string) map[string]string {
	out := make(map[string]string, len(metadata))
	for key, value := range metadata {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func metadataInt64(metadata map[string]string, keys ...string) (int64, bool) {
	for _, key := range keys {
		value := strings.TrimSpace(metadata[key])
		if value == "" {
			continue
		}
		id, err := strconv.ParseInt(value, 10, 64)
		if err == nil && id > 0 {
			return id, true
		}
	}
	return 0, false
}

func metadataFloat64(metadata map[string]string, keys ...string) (float64, bool) {
	for _, key := range keys {
		value := strings.TrimSpace(metadata[key])
		if value == "" {
			continue
		}
		score, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return score, true
		}
	}
	return 0, false
}

var _ Planner = DefaultQueryPlanner{}
