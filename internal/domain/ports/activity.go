package ports

import "context"

const (
	ActorTypeChannel  = "channel"
	ActorTypeAIAgent  = "ai_agent"
	ActorTypeService  = "service"
	ActorTypeSystem   = "system"
	ActivityCreate    = "create"
	ActivityUpdate    = "update"
	ActivityDelete    = "delete"
)

type ActivityEntry struct {
	CompanyID     int64
	ActorType     string
	ActorID       int64
	ActorLabel    string
	ActorExternal string
	SubjectType   string
	SubjectID     int64
	SubjectLabel  string
	Action        string
	Context       map[string]any
}

type ActivityRecorder interface {
	RecordActivity(ctx context.Context, e ActivityEntry) error
}
