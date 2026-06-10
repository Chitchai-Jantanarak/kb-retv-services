package main

import (
	"reflect"
	"testing"

	"github.com/my/app/internal/ai/embeddings"
)

func TestPollStateFallsBackOnEmpty(t *testing.T) {
	cases := map[string]string{
		"":                    "POLL_ERROR",
		"   ":                 "POLL_ERROR",
		"JOB_STATE_SUCCEEDED": "JOB_STATE_SUCCEEDED",
		"JOB_STATE_PENDING":   "JOB_STATE_PENDING",
	}
	for in, want := range cases {
		if got := pollState(in); got != want {
			t.Fatalf("pollState(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseCompanies(t *testing.T) {
	cases := []struct {
		name   string
		csv    string
		single int64
		want   []int64
	}{
		{name: "csv", csv: "1,2,3", single: 0, want: []int64{1, 2, 3}},
		{name: "csv trims and skips blanks", csv: " 1 , ,2 ", single: 9, want: []int64{1, 2}},
		{name: "csv overrides single", csv: "5", single: 9, want: []int64{5}},
		{name: "single fallback", csv: "", single: 7, want: []int64{7}},
		{name: "none", csv: "", single: 0, want: nil},
		{name: "skips non-numeric", csv: "a,2", single: 0, want: []int64{2}},
		{name: "skips non-positive", csv: "0,-1,3", single: 0, want: []int64{3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseCompanies(tc.csv, tc.single); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseCompanies(%q, %d) = %v, want %v", tc.csv, tc.single, got, tc.want)
			}
		})
	}
}

func TestChunkRequests(t *testing.T) {
	reqs := make([]embeddings.BatchEmbedRequest, 5)
	for i := range reqs {
		reqs[i] = embeddings.BatchEmbedRequest{Key: "k"}
	}

	single := chunkRequests(reqs, 0)
	if len(single) != 1 || len(single[0]) != 5 {
		t.Fatalf("maxPerJob=0 should yield one job of 5, got %d jobs", len(single))
	}

	split := chunkRequests(reqs, 2)
	if len(split) != 3 {
		t.Fatalf("maxPerJob=2 over 5 reqs should yield 3 jobs, got %d", len(split))
	}
	if len(split[0]) != 2 || len(split[1]) != 2 || len(split[2]) != 1 {
		t.Fatalf("split sizes = %d/%d/%d, want 2/2/1", len(split[0]), len(split[1]), len(split[2]))
	}
}

func TestEffectiveLimit(t *testing.T) {
	if got := effectiveLimit(10, 5, 100); got != 10 {
		t.Fatalf("limit should win: got %d, want 10", got)
	}
	if got := effectiveLimit(0, 5, 100); got != 5 {
		t.Fatalf("maxChunks should win when limit=0: got %d, want 5", got)
	}
	if got := effectiveLimit(0, 0, 100); got != 100 {
		t.Fatalf("config default should win: got %d, want 100", got)
	}
}

func TestIsRemoteProvider(t *testing.T) {
	for _, local := range []string{"hash", "ollama", "local"} {
		if isRemoteProvider(local) {
			t.Fatalf("%q should be local", local)
		}
	}
	for _, remote := range []string{"gemini", "openai", "voyage", "openrouter"} {
		if !isRemoteProvider(remote) {
			t.Fatalf("%q should be remote", remote)
		}
	}
}

func TestToKeyedVectors(t *testing.T) {
	kvs := toKeyedVectors([]embeddings.BatchEmbedResult{
		{Key: "2:10", Values: []float32{0.1, 0.2}},
	})
	if len(kvs) != 1 || kvs[0].Key != "2:10" || len(kvs[0].Vector) != 2 {
		t.Fatalf("toKeyedVectors mapped wrong: %+v", kvs)
	}
}
