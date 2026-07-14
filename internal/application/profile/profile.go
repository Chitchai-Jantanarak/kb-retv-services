package profile

import (
	"context"
	"fmt"
	"strings"
)

type Data struct {
	Name         string
	Website      string
	CompanySite  string
	ReplyTone    string
	Signature    string
	Products     []string
	ProblemTypes []string
}

func (d Data) Block() string {
	var b strings.Builder
	name := strings.TrimSpace(d.Name)
	if name == "" {
		name = "this company"
	}
	website := strings.TrimSpace(firstNonEmpty(d.Website, d.CompanySite))
	if website != "" {
		fmt.Fprintf(&b, "You are the support assistant for %s (%s).\n", name, website)
	} else {
		fmt.Fprintf(&b, "You are the support assistant for %s.\n", name)
	}
	if products := cleanList(d.Products); len(products) > 0 {
		fmt.Fprintf(&b, "Supported products: %s.\n", strings.Join(products, ", "))
	}
	if types := cleanList(d.ProblemTypes); len(types) > 0 {
		fmt.Fprintf(&b, "Known problem categories: %s.\n", strings.Join(types, ", "))
	}
	if tone := strings.TrimSpace(d.ReplyTone); tone != "" {
		fmt.Fprintf(&b, "Reply tone: %s.\n", tone)
	}
	if sig := strings.TrimSpace(d.Signature); sig != "" {
		fmt.Fprintf(&b, "Sign replies as: %s.\n", sig)
	}
	return strings.TrimRight(b.String(), "\n")
}

type Repository interface {
	Load(ctx context.Context, companyID int64) (Data, error)
}

type Assembler struct {
	repo Repository
}

func NewAssembler(repo Repository) *Assembler {
	return &Assembler{repo: repo}
}

func (a *Assembler) Build(ctx context.Context, companyID int64) (string, error) {
	if a == nil || a.repo == nil {
		return "", nil
	}
	data, err := a.repo.Load(ctx, companyID)
	if err != nil {
		return "", err
	}
	return data.Block(), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}
	return out
}
