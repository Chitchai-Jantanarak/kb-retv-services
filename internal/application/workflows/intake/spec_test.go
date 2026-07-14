package intake_test

import (
	"reflect"
	"testing"

	"github.com/my/app/internal/application/workflows/intake"
)

func TestDefaultSpecRequiresProblemDetailAndProduct(t *testing.T) {
	spec := intake.DefaultSpec()
	if !reflect.DeepEqual(spec.Required, []string{"problem_detail", "product"}) {
		t.Fatalf("Required = %v", spec.Required)
	}
	if !reflect.DeepEqual(spec.Optional, []string{"contact", "severity"}) {
		t.Fatalf("Optional = %v", spec.Optional)
	}
}

func TestSpecMissingListsBlankRequiredFields(t *testing.T) {
	spec := intake.DefaultSpec()
	missing := spec.Missing(map[string]string{"problem_detail": "printer jams", "product": "   "})
	if !reflect.DeepEqual(missing, []string{"product"}) {
		t.Fatalf("Missing = %v, want [product]", missing)
	}
	if got := spec.StatusFor(map[string]string{"problem_detail": "x", "product": ""}); got != intake.StatusIncomplete {
		t.Fatalf("StatusFor = %q, want incomplete", got)
	}
	if got := spec.StatusFor(map[string]string{"problem_detail": "x", "product": "Bella Bot"}); got != intake.StatusReady {
		t.Fatalf("StatusFor = %q, want ready", got)
	}
}

func TestSpecNormalizedFallsBackWhenNoRequiredFields(t *testing.T) {
	spec := intake.Spec{Optional: []string{"contact"}}.Normalized()
	if !reflect.DeepEqual(spec.Required, intake.DefaultSpec().Required) {
		t.Fatalf("Required = %v, want default", spec.Required)
	}
	if !reflect.DeepEqual(spec.Optional, []string{"contact"}) {
		t.Fatalf("Optional = %v", spec.Optional)
	}
}

func TestSpecNormalizedDedupesAndDropsOptionalDuplicatesOfRequired(t *testing.T) {
	spec := intake.Spec{
		Required: []string{"Product", " product ", "problem_detail"},
		Optional: []string{"product", "contact"},
	}.Normalized()
	if !reflect.DeepEqual(spec.Required, []string{"product", "problem_detail"}) {
		t.Fatalf("Required = %v", spec.Required)
	}
	if !reflect.DeepEqual(spec.Optional, []string{"contact"}) {
		t.Fatalf("Optional = %v", spec.Optional)
	}
}

func TestSpecFromDefinitionsSplitsRequiredAndOptional(t *testing.T) {
	spec, custom := intake.SpecFromDefinitions([]intake.FieldDefinition{
		{Key: "problem_detail", Required: true},
		{Key: "site", Required: true},
		{Key: "contact"},
		{Key: "  ", Required: true},
	})
	if !custom {
		t.Fatal("custom = false, want true")
	}
	if !reflect.DeepEqual(spec.Required, []string{"problem_detail", "site"}) {
		t.Fatalf("Required = %v", spec.Required)
	}
	if !reflect.DeepEqual(spec.Optional, []string{"contact"}) {
		t.Fatalf("Optional = %v", spec.Optional)
	}
}

func TestSpecFromDefinitionsEmptyReturnsDefault(t *testing.T) {
	spec, custom := intake.SpecFromDefinitions(nil)
	if custom {
		t.Fatal("custom = true, want false")
	}
	if !reflect.DeepEqual(spec.Required, intake.DefaultSpec().Required) {
		t.Fatalf("Required = %v, want default", spec.Required)
	}
}

func TestLineageIsNearestFirstAndDepthCapped(t *testing.T) {
	got := intake.Lineage("/1/2/3/4/5/", 5, intake.LineageMaxAncestors)
	if !reflect.DeepEqual(got, []int64{5, 4, 3, 2}) {
		t.Fatalf("Lineage = %v, want [5 4 3 2]", got)
	}
}

func TestLineageFallsBackToSelfWhenPathEmpty(t *testing.T) {
	if got := intake.Lineage("", 9, 4); !reflect.DeepEqual(got, []int64{9}) {
		t.Fatalf("Lineage = %v, want [9]", got)
	}
}
