package ports

import "context"

type AIActionSource struct {
	ID        string  `json:"id"`
	Title     string  `json:"title,omitempty"`
	Score     float64 `json:"score,omitempty"`
	Retriever string  `json:"retriever,omitempty"`
}

type AIAction struct {
	CompanyID  int64
	AgentID    int64
	ActionType string
	Input      any
	Output     any
	TokensIn   int
	TokensOut  int
	LatencyMs  int
	Confidence float64
	Err        string
}

type AIActionRecorder interface {
	Record(ctx context.Context, action AIAction) (int64, error)
}

type AIActionFeedback struct {
	CompanyID int64
	ActionID  int64
	Verdict   string
	Score     int8
	Note      string
}

type AIActionFeedbackRecorder interface {
	RecordFeedback(ctx context.Context, fb AIActionFeedback) error
}
