package embeddings

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
)

type BatchEmbedRequest struct {
	Key  string
	Text string
}

type BatchEmbedResult struct {
	Key    string
	Values []float32
}

type batchEmbedLine struct {
	Key     string            `json:"key"`
	Request batchEmbedRequest `json:"request"`
}

type batchEmbedRequest struct {
	Model                string       `json:"model"`
	Content              batchContent `json:"content"`
	OutputDimensionality int          `json:"outputDimensionality,omitempty"`
}

type batchContent struct {
	Parts []batchPart `json:"parts"`
}

type batchPart struct {
	Text string `json:"text"`
}

func buildEmbedJSONL(model string, dim int, reqs []BatchEmbedRequest) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, req := range reqs {
		line := batchEmbedLine{
			Key: req.Key,
			Request: batchEmbedRequest{
				Model:                "models/" + model,
				Content:              batchContent{Parts: []batchPart{{Text: req.Text}}},
				OutputDimensionality: dim,
			},
		}
		_ = enc.Encode(line)
	}
	return buf.Bytes()
}

func parseEmbedResultsJSONL(data []byte) ([]BatchEmbedResult, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)

	var results []BatchEmbedResult
	for scanner.Scan() {
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}

		var line struct {
			Key      string `json:"key"`
			Response struct {
				Embedding struct {
					Values []float32 `json:"values"`
				} `json:"embedding"`
				Status *struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"status"`
				Error *struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"response"`
		}
		if err := json.Unmarshal(raw, &line); err != nil {
			return nil, err
		}

		if line.Response.Status != nil && line.Response.Status.Code != 0 {
			return nil, fmt.Errorf("batch embedding failed for key %s: %s", line.Key, line.Response.Status.Message)
		}
		if line.Response.Error != nil {
			return nil, fmt.Errorf("batch embedding failed for key %s: %s", line.Key, line.Response.Error.Message)
		}

		results = append(results, BatchEmbedResult{Key: line.Key, Values: line.Response.Embedding.Values})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return results, nil
}
