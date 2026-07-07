package tools

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
)

var validKinds = map[string]bool{"read": true, "retrieval": true, "write": true}

func Load(fsys fs.FS, validPerms map[string]bool) ([]Tool, error) {
	names, err := fs.Glob(fsys, "*.json")
	if err != nil {
		return nil, fmt.Errorf("tools: glob: %w", err)
	}

	tools := make([]Tool, 0, len(names))
	for _, name := range names {
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("tools: read %s: %w", name, err)
		}

		var t Tool
		if err := json.Unmarshal(raw, &t); err != nil {
			return nil, fmt.Errorf("tools: parse %s: %w", name, err)
		}

		if err := validate(t, name, validPerms); err != nil {
			return nil, err
		}

		tools = append(tools, t)
	}

	sort.Slice(tools, func(i, j int) bool { return tools[i].ID < tools[j].ID })

	return tools, nil
}

func validate(t Tool, name string, validPerms map[string]bool) error {
	if t.ID == "" {
		return fmt.Errorf("tools: %s: missing id", name)
	}
	if t.Intent == "" {
		return fmt.Errorf("tools: %s: missing intent", name)
	}
	if t.Handler == "" {
		return fmt.Errorf("tools: %s: missing handler", name)
	}
	if !validKinds[t.Kind] {
		return fmt.Errorf("tools: %s: invalid kind %q", name, t.Kind)
	}
	if t.RBAC.RequiresPermission == "" {
		return fmt.Errorf("tools: %s: missing rbac.requires_permission", name)
	}
	if len(validPerms) > 0 && !validPerms[t.RBAC.RequiresPermission] {
		return fmt.Errorf("tools: %s: unknown permission %q", name, t.RBAC.RequiresPermission)
	}

	return nil
}
