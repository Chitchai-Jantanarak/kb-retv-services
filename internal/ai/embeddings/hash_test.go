package embeddings

import (
	"context"
	"testing"
)

func TestHashProviderReturnsStableNormalizedVectors(t *testing.T) {
	provider := NewHashProvider(8)

	first, err := provider.Embed(context.Background(), []string{"printer offline", "printer offline"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if len(first) != 2 {
		t.Fatalf("vectors = %d, want 2", len(first))
	}
	if len(first[0]) != 8 {
		t.Fatalf("dim = %d, want 8", len(first[0]))
	}
	for i := range first[0] {
		if first[0][i] != first[1][i] {
			t.Fatalf("vectors differ at %d: %v != %v", i, first[0][i], first[1][i])
		}
	}
}
