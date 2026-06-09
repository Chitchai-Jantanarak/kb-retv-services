package dto

type IngestReportPayload struct {
	Limit     int  `json:"limit,omitempty"`
	BatchSize int  `json:"batch_size,omitempty"`
	DryRun    bool `json:"dry_run,omitempty"`
}
