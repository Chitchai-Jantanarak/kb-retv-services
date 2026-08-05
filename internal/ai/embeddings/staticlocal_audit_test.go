package embeddings

import "testing"

func TestValidateThresholdsRejectsUnusableValues(t *testing.T) {
	tests := []struct {
		name string
		meta staticMeta
	}{
		{"zero floor", staticMeta{Floor: 0, Accept: 0.55, Margin: 0.05}},
		{"zero margin", staticMeta{Floor: 0.35, Accept: 0.55, Margin: 0}},
		{"negative accept", staticMeta{Floor: 0.35, Accept: -1, Margin: 0.05}},
		{"accept below floor", staticMeta{Floor: 0.55, Accept: 0.35, Margin: 0.05}},
		{"accept above one", staticMeta{Floor: 0.35, Accept: 1.5, Margin: 0.05}},
	}
	for _, tt := range tests {
		if err := validateThresholds(tt.meta); err == nil {
			t.Errorf("%s: validateThresholds accepted %+v; the router would run on it", tt.name, tt.meta)
		}
	}
	ok := staticMeta{Floor: 0.35, Accept: 0.55, Margin: 0.05}
	if err := validateThresholds(ok); err != nil {
		t.Errorf("validateThresholds rejected the shipped values: %v", err)
	}
}

func TestDimensionCeilingBlocksOverflow(t *testing.T) {
	vocab, dim := 3, 6148914691236517206
	if vocab*dim != 2 {
		t.Fatalf("premise changed: vocab*dim = %d, expected the multiplication to wrap to 2", vocab*dim)
	}
	if dim <= maxDim {
		t.Fatalf("dim %d must exceed maxDim %d for the ceiling to reject it", dim, maxDim)
	}
}
