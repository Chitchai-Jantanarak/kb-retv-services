package prompts

import (
	"bufio"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

//go:embed templates/*.prompt
var embeddedTemplates embed.FS

// LoadEmbedded parses all .prompt files baked into the binary at
// internal/ai/prompts/templates/*.prompt.
func LoadEmbedded() ([]Template, error) {
	return LoadFS(embeddedTemplates, "templates")
}

// LoadFS parses every *.prompt file under dir in the given fs.FS.
// Use this to load operator-overridable prompts from disk:
//
//	os.DirFS("/etc/smartjob/prompts") + LoadFS(fsys, ".")
func LoadFS(fsys fs.FS, dir string) ([]Template, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("prompts: read dir %q: %w", dir, err)
	}

	out := make([]Template, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".prompt") {
			continue
		}
		path := dir + "/" + e.Name()
		raw, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("prompts: read %s: %w", path, err)
		}
		tmpl, err := parsePrompt(string(raw))
		if err != nil {
			return nil, fmt.Errorf("prompts: parse %s: %w", path, err)
		}
		out = append(out, tmpl)
	}
	return out, nil
}

// parsePrompt reads the simple text format used for prompt files:
//
//	@name        rewrite
//	@max_toks    120
//	@temp        0.1
//	@required    query
//	@stop        \n\n
//	@system
//	(multi-line system prompt body)
//	@user
//	(multi-line user prompt body)
//
// `@system` and `@user` consume everything until the next section
// header or EOF. Lines beginning with `#` before the first section
// are comments. Headers and section markers are case-sensitive.
func parsePrompt(src string) (Template, error) {
	var (
		t       Template
		section string
		body    strings.Builder
	)

	flush := func() error {
		if section == "" {
			return nil
		}
		text := strings.TrimRight(body.String(), "\n")
		switch section {
		case "system":
			t.System = text
		case "user":
			t.User = text
		default:
			return fmt.Errorf("unknown section %q", section)
		}
		body.Reset()
		return nil
	}

	scanner := bufio.NewScanner(strings.NewReader(src))
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if section != "" {
			// Within @system/@user, only an `@system` or `@user` line ends the section.
			if trimmed == "@system" || trimmed == "@user" {
				if err := flush(); err != nil {
					return Template{}, err
				}
				section = strings.TrimPrefix(trimmed, "@")
				continue
			}
			body.WriteString(line)
			body.WriteByte('\n')
			continue
		}

		// Pre-section header parsing.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(trimmed, "@") {
			return Template{}, fmt.Errorf("unexpected line before sections: %q", line)
		}
		if trimmed == "@system" || trimmed == "@user" {
			section = strings.TrimPrefix(trimmed, "@")
			continue
		}

		key, value, ok := splitDirective(trimmed)
		if !ok {
			return Template{}, fmt.Errorf("malformed directive: %q", line)
		}
		if err := applyDirective(&t, key, value); err != nil {
			return Template{}, err
		}
	}
	if err := scanner.Err(); err != nil {
		return Template{}, err
	}
	if err := flush(); err != nil {
		return Template{}, err
	}

	if t.Name == "" {
		return Template{}, errors.New("missing @name")
	}
	if t.User == "" {
		return Template{}, errors.New("missing @user body")
	}
	return t, nil
}

func splitDirective(line string) (string, string, bool) {
	line = strings.TrimPrefix(line, "@")
	for i, ch := range line {
		if ch == ' ' || ch == '\t' {
			return line[:i], strings.TrimSpace(line[i:]), true
		}
	}
	return line, "", true
}

func applyDirective(t *Template, key, value string) error {
	switch key {
	case "name":
		t.Name = value
	case "required":
		t.RequiredVars = splitFields(value)
	case "max_toks":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("max_toks: %w", err)
		}
		t.MaxToks = n
	case "temp":
		f, err := strconv.ParseFloat(value, 32)
		if err != nil {
			return fmt.Errorf("temp: %w", err)
		}
		t.Temp = float32(f)
	case "stop":
		t.Stop = splitFields(value)
	default:
		return fmt.Errorf("unknown directive @%s", key)
	}
	return nil
}

func splitFields(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
