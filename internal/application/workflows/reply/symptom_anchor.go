package reply

import (
	"context"
	"maps"
	"strconv"
	"strings"
)

type SymptomAnchorer interface {
	Anchor(ctx context.Context, companyID int64, message string) (symptomID int64, confidence float64)
}

func applySymptomAnchor(ctx context.Context, anchorer SymptomAnchorer, companyID int64, message string, metadata map[string]string) map[string]string {
	if anchorer == nil || hasConfirmedSymptom(metadata) {
		return metadata
	}

	symptomID, confidence := anchorer.Anchor(ctx, companyID, message)
	if symptomID <= 0 {
		return metadata
	}

	out := make(map[string]string, len(metadata)+2)
	maps.Copy(out, metadata)
	out["ai_symptom_node_id"] = strconv.FormatInt(symptomID, 10)
	out["ai_symptom_conf"] = strconv.FormatFloat(confidence, 'f', -1, 64)
	return out
}

func hasConfirmedSymptom(metadata map[string]string) bool {
	for key, value := range metadata {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if (normalized == "symptom_node_id" || normalized == "symptom_id") && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}
