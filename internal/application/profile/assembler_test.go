package profile_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/my/app/internal/application/profile"
)

type fakeRepo struct {
	data profile.Data
	err  error
}

func (f fakeRepo) Load(context.Context, int64) (profile.Data, error) {
	return f.data, f.err
}

func TestAssemblerBuildIncludesIdentityProductsAndTone(t *testing.T) {
	a := profile.NewAssembler(fakeRepo{data: profile.Data{
		Name:         "Acme",
		Website:      "acme.io",
		Products:     []string{"Router X", "Sensor Y", "Router X"},
		ProblemTypes: []string{"connectivity"},
		ReplyTone:    "concise",
		Signature:    "Acme Support",
	}})

	out, err := a.Build(context.Background(), 7)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	for _, want := range []string{"Acme", "acme.io", "Router X", "Sensor Y", "connectivity", "concise", "Acme Support"} {
		if !strings.Contains(out, want) {
			t.Fatalf("block missing %q\n%s", want, out)
		}
	}
	if strings.Count(out, "Router X") != 1 {
		t.Fatalf("Router X should appear once (deduped)\n%s", out)
	}
}

func TestAssemblerBuildSkipsEmptyFields(t *testing.T) {
	a := profile.NewAssembler(fakeRepo{data: profile.Data{Name: "Acme"}})
	out, err := a.Build(context.Background(), 7)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if strings.Contains(out, "Supported products") || strings.Contains(out, "Reply tone") {
		t.Fatalf("empty fields must not render a line\n%s", out)
	}
	if !strings.Contains(out, "Acme") {
		t.Fatalf("identity line missing\n%s", out)
	}
}

func TestAssemblerBuildNilRepoReturnsEmpty(t *testing.T) {
	var a *profile.Assembler
	out, err := a.Build(context.Background(), 7)
	if err != nil || out != "" {
		t.Fatalf("nil assembler Build() = (%q, %v), want empty", out, err)
	}
}

func TestAssemblerBuildPropagatesError(t *testing.T) {
	a := profile.NewAssembler(fakeRepo{err: errors.New("boom")})
	if _, err := a.Build(context.Background(), 7); err == nil {
		t.Fatal("Build() error = nil, want propagated repo error")
	}
}
