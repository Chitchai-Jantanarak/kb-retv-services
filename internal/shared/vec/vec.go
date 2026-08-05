package vec

import "math"

func Normalize(vec []float32) []float32 {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return vec
	}
	norm := math.Sqrt(sum)
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = float32(float64(v) / norm)
	}
	return out
}

func NormalizeInPlace(vec []float32) {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return
	}
	norm := math.Sqrt(sum)
	for i, v := range vec {
		vec[i] = float32(float64(v) / norm)
	}
}

func Dot(a, b []float32) float64 {
	n := min(len(a), len(b))
	var sum float64
	for i := range n {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}
