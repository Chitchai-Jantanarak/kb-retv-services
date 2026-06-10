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
	results, failed, err := parseEmbedResultsJSONL([]byte(body))
	if err != nil {
		t.Fatalf("parse error = %v", err)
	}
	if len(failed) != 0 {
		t.Fatalf("failed = %d, want 0", len(failed))
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
	results, _, err := parseEmbedResultsJSONL([]byte(body))
	if err != nil {
		t.Fatalf("parse error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
}

func TestParseEmbedResultsReturnsPartialFailures(t *testing.T) {
	body := `{"key":"10","response":{"embedding":{"values":[0.1,0.2,0.3]}}}
{"key":"9","response":{"status":{"code":3,"message":"bad input"}}}
{"key":"8","response":{"error":{"message":"quota"}}}
{"key":"11","response":{"embedding":{"values":[0.4,0.5,0.6]}}}
`
	results, failed, err := parseEmbedResultsJSONL([]byte(body))
	if err != nil {
		t.Fatalf("parse error = %v, want nil (per-key failures are not hard errors)", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2 good", len(results))
	}
	if len(failed) != 2 {
		t.Fatalf("failed = %d, want 2", len(failed))
	}
	keys := map[string]string{failed[0].Key: failed[0].Message, failed[1].Key: failed[1].Message}
	if _, ok := keys["9"]; !ok {
		t.Fatalf("failed should include key 9, got %+v", failed)
	}
	if _, ok := keys["8"]; !ok {
		t.Fatalf("failed should include key 8, got %+v", failed)
	}
}

func TestParseEmbedResultsSkipsEmptyValues(t *testing.T) {
	body := `{"key":"7","response":{"embedding":{"values":[]}}}` + "\n"
	results, failed, err := parseEmbedResultsJSONL([]byte(body))
	if err != nil {
		t.Fatalf("parse error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("results = %d, want 0 (empty values must not become a 0-dim point)", len(results))
	}
	if len(failed) != 1 || failed[0].Key != "7" {
		t.Fatalf("failed = %+v, want key 7", failed)
	}
}

func TestParseEmbedResultsHardErrorsOnMalformedJSON(t *testing.T) {
	body := `{"key":"5","response":{` + "\n"
	if _, _, err := parseEmbedResultsJSONL([]byte(body)); err == nil {
		t.Fatal("expected hard error for malformed JSON line, got nil")
	}
}
