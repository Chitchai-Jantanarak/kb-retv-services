package embeddings

import (
	"context"
	"hash/fnv"
	"strings"

	"github.com/my/app/internal/shared/vec"
)

type HashProvider struct {
	dim int
}

func NewHashProvider(dim int) *HashProvider {
	if dim <= 0 {
		dim = 384
	}
	return &HashProvider{dim: dim}
}

func (p *HashProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))
	for _, text := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		vectors = append(vectors, p.embedOne(text))
	}
	return vectors, nil
}

func (p *HashProvider) Dim() int {
	return p.dim
}

func (p *HashProvider) embedOne(text string) []float32 {
	vector := make([]float32, p.dim)
	for _, term := range strings.Fields(strings.ToLower(text)) {
		index := hashTerm(term) % uint64(p.dim)
		vector[index]++
	}
	vec.NormalizeInPlace(vector)
	return vector
}

func hashTerm(term string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(term))
	return h.Sum64()
}
