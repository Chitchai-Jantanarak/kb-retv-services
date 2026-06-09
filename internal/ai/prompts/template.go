package prompts

import (
	"fmt"
	"sort"
	"strings"

	"github.com/my/app/internal/domain/ports"
)

// Template is a named prompt scaffold. Variables in System/User are
// referenced as {{name}} and must be listed in RequiredVars. Render
// fails fast when any required var is missing or empty so callers
// never silently send under-templated prompts to a paid LLM.
type Template struct {
	Name         string
	System       string
	User         string
	RequiredVars []string
	MaxToks      int
	Temp         float32
	Stop         []string
}

func (t Template) Render(vars map[string]string) (ports.Prompt, error) {
	missing := make([]string, 0)
	for _, name := range t.RequiredVars {
		v, ok := vars[name]
		if !ok || strings.TrimSpace(v) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return ports.Prompt{}, fmt.Errorf("prompts: template %q missing required vars: %s",
			t.Name, strings.Join(missing, ", "))
	}

	system := substitute(t.System, vars)
	user := substitute(t.User, vars)

	return ports.Prompt{
		System:  system,
		User:    user,
		Vars:    vars,
		MaxToks: t.MaxToks,
		Temp:    t.Temp,
		Stop:    t.Stop,
	}, nil
}

func substitute(in string, vars map[string]string) string {
	out := in
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}
