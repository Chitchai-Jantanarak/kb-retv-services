package embeddings

import (
	"strings"
	"testing"
)

func TestBuildEmbedJSONLOneLinePerRequest(t *testing.T) {
	reqs := []BatchEmbedRequest{
		{Key: "10", Text: "hello"},
		{Key: "11", Text: "world"},
	}

	data := buildEmbedJSONL("gemini-embedding-2", 3072, reqs)

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	first := lines[0]
	for _, want := range []string{
		`"key":"10"`,
		`"model":"models/gemini-embedding-2"`,
		`"text":"hello"`,
		`"outputDimensionality":3072`,
	} {
		if !strings.Contains(first, want) {
			t.Fatalf("line %q missing %q", first, want)
		}
	}
}

func TestBuildEmbedJSONLOmitsDimensionWhenZero(t *testing.T) {
	data := buildEmbedJSONL("gemini-embedding-2", 0, []BatchEmbedRequest{{Key: "1", Text: "x"}})
	if strings.Contains(string(data), "outputDimensionality") {
		t.Fatalf("expected no outputDimensionality when dim=0, got %s", data)
	}
}

func TestParseEmbedResultsJSONL(t *testing.T) {
	body := `{"key":"10","response":{"embedding":{"values":[0.1,0.2,0.3]}}}
{"key":"11","response":{"embedding":{"values":[0.4,0.5,0.6]}}}
`
	results, err := parseEmbedResultsJSONL([]byte(body))
	if err != nil {
		t.Fatalf("parse error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if results[0].Key != "10" || results[0].Values[0] != 0.1 {
		t.Fatalf("results[0] = %+v, want key=10 first=0.1", results[0])
	}
	if results[1].Key != "11" || results[1].Values[2] != 0.6 {
		t.Fatalf("results[1] = %+v, want key=11 last=0.6", results[1])
	}
}

func TestParseEmbedResultsJSONLSkipsBlankLines(t *testing.T) {
	body := "\n{\"key\":\"1\",\"response\":{\"embedding\":{\"values\":[1.0]}}}\n\n"
	results, err := parseEmbedResultsJSONL([]byte(body))
	if err != nil {
		t.Fatalf("parse error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
}

func TestParseEmbedResultsJSONLReportsErrorLine(t *testing.T) {
	body := `{"key":"9","response":{"status":{"code":3,"message":"bad"}}}` + "\n"
	_, err := parseEmbedResultsJSONL([]byte(body))
	if err == nil {
		t.Fatal("expected error for result line carrying an error status, got nil")
	}
	if !strings.Contains(err.Error(), "9") {
		t.Fatalf("error %v should name the failing key 9", err)
	}
}
