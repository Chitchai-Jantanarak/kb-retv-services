package intake

import (
	"context"
	"strconv"
	"strings"
)

const (
	FieldProblemDetail = "problem_detail"
	FieldProduct       = "product"
	FieldContact       = "contact"
	FieldSeverity      = "severity"
)

const (
	StatusUnknown    = "unknown"
	StatusReady      = "ready"
	StatusIncomplete = "incomplete"
	StatusUnverified = "unverified"
)

const (
	EntityType        = "report_intake"
	LineageMaxAncestors = 4
)

type Spec struct {
	Required []string
	Optional []string
}

func DefaultSpec() Spec {
	return Spec{
		Required: []string{FieldProblemDetail, FieldProduct},
		Optional: []string{FieldContact, FieldSeverity},
	}
}

func (s Spec) Normalized() Spec {
	required := cleanKeys(s.Required)
	if len(required) == 0 {
		required = DefaultSpec().Required
	}
	optional := make([]string, 0, len(s.Optional))
	for _, key := range cleanKeys(s.Optional) {
		if !contains(required, key) {
			optional = append(optional, key)
		}
	}
	return Spec{Required: required, Optional: optional}
}

func (s Spec) Fields() []string {
	spec := s.Normalized()
	out := make([]string, 0, len(spec.Required)+len(spec.Optional))
	out = append(out, spec.Required...)
	out = append(out, spec.Optional...)
	return out
}

func (s Spec) Missing(fields map[string]string) []string {
	spec := s.Normalized()
	missing := make([]string, 0, len(spec.Required))
	for _, key := range spec.Required {
		if strings.TrimSpace(fields[key]) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

func (s Spec) StatusFor(fields map[string]string) string {
	if len(s.Missing(fields)) == 0 {
		return StatusReady
	}
	return StatusIncomplete
}

type SpecResolver interface {
	SpecFor(ctx context.Context, companyID int64) (Spec, error)
}

type FieldDefinition struct {
	Key      string
	Required bool
}

func SpecFromDefinitions(defs []FieldDefinition) (Spec, bool) {
	var spec Spec
	for _, def := range defs {
		key := strings.ToLower(strings.TrimSpace(def.Key))
		if key == "" {
			continue
		}
		if def.Required {
			spec.Required = append(spec.Required, key)
			continue
		}
		spec.Optional = append(spec.Optional, key)
	}
	if len(spec.Required) == 0 && len(spec.Optional) == 0 {
		return DefaultSpec(), false
	}
	return spec.Normalized(), true
}

func Lineage(path string, companyID int64, max int) []int64 {
	ids := make([]int64, 0, 4)
	for _, part := range strings.Split(path, "/") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		if companyID <= 0 {
			return nil
		}
		return []int64{companyID}
	}
	reversed := make([]int64, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		reversed = append(reversed, ids[i])
	}
	if max > 0 && len(reversed) > max {
		reversed = reversed[:max]
	}
	return reversed
}

func cleanKeys(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, key := range in {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
