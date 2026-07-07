package embeddings

import "context"

type tokenizerEncoder interface {
	EncodeIDs(text string) []uint32
}

type StaticLocalProvider struct {
	table *staticTable
	enc   tokenizerEncoder
}

func (p *StaticLocalProvider) Dim() int { return p.table.dim }

func (p *StaticLocalProvider) Thresholds() (floor, accept, margin float64) {
	return p.table.floor, p.table.accept, p.table.margin
}

func (p *StaticLocalProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out = append(out, p.embedIDs(p.enc.EncodeIDs(text)))
	}
	return out, nil
}

func (p *StaticLocalProvider) embedIDs(ids []uint32) []float32 {
	sum := make([]float32, p.table.dim)
	count := 0
	for _, id := range ids {
		row := p.table.row(int(id))
		for i := range sum {
			sum[i] += row[i]
		}
		count++
	}
	if count == 0 {
		copy(sum, p.table.row(p.table.unkID))
		count = 1
	}
	inv := 1.0 / float32(count)
	for i := range sum {
		sum[i] *= inv
	}
	normalize(sum)
	return sum
}
