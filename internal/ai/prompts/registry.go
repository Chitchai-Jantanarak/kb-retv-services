package prompts

import (
	"fmt"
	"io/fs"
	"sort"
)

type Registry struct {
	templates map[string]Template
}

// NewRegistry loads the canonical template set embedded in the binary.
// To override prompts at runtime without rebuilding, use NewRegistryFromFS
// against an os.DirFS pointed at your overrides directory.
func NewRegistry() (*Registry, error) {
	tmpls, err := LoadEmbedded()
	if err != nil {
		return nil, err
	}
	return newRegistryFrom(tmpls)
}

// NewRegistryFromFS builds a Registry from any fs.FS containing
// *.prompt files under dir. Useful for tests and operator overrides.
func NewRegistryFromFS(fsys fs.FS, dir string) (*Registry, error) {
	tmpls, err := LoadFS(fsys, dir)
	if err != nil {
		return nil, err
	}
	return newRegistryFrom(tmpls)
}

// MustNewRegistry panics on load failure. Use only in `main` or test
// init where a missing embedded prompt is a build-time bug.
func MustNewRegistry() *Registry {
	r, err := NewRegistry()
	if err != nil {
		panic("prompts: " + err.Error())
	}
	return r
}

func newRegistryFrom(tmpls []Template) (*Registry, error) {
	r := &Registry{templates: make(map[string]Template, len(tmpls))}
	for _, t := range tmpls {
		if _, dup := r.templates[t.Name]; dup {
			return nil, fmt.Errorf("prompts: duplicate template name %q", t.Name)
		}
		r.templates[t.Name] = t
	}
	missing := make([]string, 0)
	for _, want := range canonicalNames() {
		if _, ok := r.templates[want]; !ok {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("prompts: missing canonical templates: %v", missing)
	}
	return r, nil
}

func (r *Registry) Get(name string) (Template, error) {
	t, ok := r.templates[name]
	if !ok {
		return Template{}, fmt.Errorf("prompts: unknown template %q", name)
	}
	return t, nil
}

func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.templates))
	for name := range r.templates {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Defaults returns every template loaded by NewRegistry. Kept for
// backwards-compatible test surface.
func Defaults() []Template {
	r, err := NewRegistry()
	if err != nil {
		panic("prompts: load embedded defaults: " + err.Error())
	}
	out := make([]Template, 0, len(r.templates))
	for _, name := range r.Names() {
		out = append(out, r.templates[name])
	}
	return out
}
