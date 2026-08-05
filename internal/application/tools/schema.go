package tools

type Tool struct {
	ID          string            `json:"id"`
	Intent      string            `json:"intent"`
	Kind        string            `json:"kind"`
	Description string            `json:"description"`
	Anchors     []string          `json:"anchors"`
	Thresholds  Thresholds        `json:"thresholds"`
	Params      []Param           `json:"params"`
	Resolve     []ResolveRule     `json:"resolve"`
	Defaults    map[string]string `json:"defaults"`
	RBAC        RBAC              `json:"rbac"`
	Handler     string            `json:"handler"`
	Limit       int               `json:"limit"`
	Compose     Compose           `json:"compose"`
	Audit       string            `json:"audit"`
	EnabledBy   string            `json:"enabled_by"`
}

type Param struct {
	Name     string            `json:"name"`
	Required bool              `json:"required"`
	Pattern  string            `json:"pattern,omitempty"`
	Map      map[string]string `json:"map,omitempty"`
	Lookup   string            `json:"lookup,omitempty"`
}

func (t Tool) IsWrite() bool { return t.Kind == "write" }

type Thresholds struct {
	Accept float64 `json:"accept"`
	Floor  float64 `json:"floor"`
}

type ResolveRule struct {
	Word string            `json:"word"`
	To   string            `json:"to"`
	Via  string            `json:"via,omitempty"`
	Map  map[string]string `json:"map,omitempty"`
}

type RBAC struct {
	RequiresPermission string `json:"requires_permission"`
	TeamView           bool   `json:"team_view"`
}

type Compose struct {
	Mode     string       `json:"mode"`
	Headline string       `json:"headline"`
	Row      string       `json:"row"`
	Cite     string       `json:"cite"`
	Columns  []ToolColumn `json:"columns"`
}

type ToolColumn struct {
	Key     string `json:"key"`
	Type    string `json:"type"`
	Primary bool   `json:"primary,omitempty"`
}
