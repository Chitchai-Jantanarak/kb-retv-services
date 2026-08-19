package intake

import "strings"

const ConfidencePromoteThreshold = 55

const (
	confBaseNewIssue      = 60
	confBaseFollowUp      = 60
	confBaseStatusQuery   = 35
	confBaseUnclear       = 25
	confBaseNotActionable = 10
	confBaseDefault       = 30

	confJunkPenalty  = -25
	confCleanBonus   = 5
	confRefCaseBonus = 20
	confFieldsBonus  = 8
	confCatalogBonus = 7
	confCatalogVeto  = -10

	confFloor = 5
	confCeil  = 95
)

var confJunkStrong = map[string]struct{}{
	ReasonListUnsubscribe: {},
	ReasonAutoSubmitted:   {},
	ReasonSenderNoReply:   {},
}

var confPositive = map[string]struct{}{
	ReasonReferencedCase: {},
	ReasonThreadMatched:  {},
	ReasonHasAttachments: {},
	ReasonSenderKnown:    {},
}

func Confidence(score int, reasons []string, classification string, missing []string, catalogRelated *bool, referencedCase string) (int, []string) {
	trail := make([]string, 0, 6)

	conf := confBaseDefault
	switch classification {
	case ClassificationNewIssue:
		conf = confBaseNewIssue
	case ClassificationFollowUp:
		conf = confBaseFollowUp
	case ClassificationStatusQuery:
		conf = confBaseStatusQuery
	case ClassificationUnclear:
		conf = confBaseUnclear
	case ClassificationNotActionable:
		conf = confBaseNotActionable
	}
	baseTag := strings.TrimSpace(classification)
	if baseTag == "" {
		baseTag = "none"
	}
	trail = append(trail, "base:"+baseTag)

	hasJunk, hasPos := false, false
	for _, r := range reasons {
		if _, ok := confJunkStrong[r]; ok {
			hasJunk = true
		}
		if _, ok := confPositive[r]; ok {
			hasPos = true
		}
	}
	switch {
	case hasJunk:
		conf += confJunkPenalty
		trail = append(trail, "t0_conflict")
	case hasPos || score >= 50:
		conf += confCleanBonus
		trail = append(trail, "t0_clean")
	}

	if strings.TrimSpace(referencedCase) != "" {
		conf += confRefCaseBonus
		trail = append(trail, "linked_case")
	}
	if len(missing) == 0 {
		conf += confFieldsBonus
		trail = append(trail, "fields_complete")
	}
	if catalogRelated != nil {
		if *catalogRelated {
			conf += confCatalogBonus
			trail = append(trail, "catalog_match")
		} else {
			conf += confCatalogVeto
			trail = append(trail, "catalog_veto")
		}
	}

	if conf < confFloor {
		conf = confFloor
	}
	if conf > confCeil {
		conf = confCeil
	}
	return conf, trail
}

func ConfidencePromotable(classification string, confidence int) bool {
	if confidence < ConfidencePromoteThreshold {
		return false
	}
	return classification == ClassificationNewIssue || classification == ClassificationFollowUp
}
