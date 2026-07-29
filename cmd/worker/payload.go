package main

import (
	"encoding/json"
	"fmt"

	"github.com/my/app/internal/application/dto"
)

func decodeIngestPayload(raw []byte) (dto.IngestReportPayload, error) {
	var payload dto.IngestReportPayload
	if len(raw) == 0 {
		return payload, nil
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload, fmt.Errorf("decode payload: %w", err)
	}
	return payload, nil
}
