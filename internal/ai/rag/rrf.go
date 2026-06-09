package rag

import "sort"

const rrfK = 60

func rrfMerge(sets [][]Candidate) []Candidate {
	type entry struct {
		candidate Candidate
		rrfScore  float64
	}

	merged := make(map[string]*entry)
	for _, set := range sets {
		for rank, c := range set {
			if c.ID == "" {
				continue
			}
			inc := 1.0 / float64(rrfK+rank+1)
			if e, ok := merged[c.ID]; ok {
				e.rrfScore += inc
				if c.Score > e.candidate.Score {
					e.candidate = c
				}
			} else {
				merged[c.ID] = &entry{candidate: c, rrfScore: inc}
			}
		}
	}

	out := make([]Candidate, 0, len(merged))
	for _, e := range merged {
		c := e.candidate
		c.Score = e.rrfScore
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	return out
}
