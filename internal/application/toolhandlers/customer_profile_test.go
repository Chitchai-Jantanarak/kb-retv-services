package toolhandlers

import (
	"context"
	"testing"

	"github.com/my/app/internal/application/skeleton"
	customermysql "github.com/my/app/internal/repositories/customer/mysql"
)

type stubCustomerRepo struct {
	profile customermysql.Profile
	covered []int64
	name    string
}

func (s *stubCustomerRepo) ProfileByName(_ context.Context, coverage []int64, name string) (customermysql.Profile, error) {
	s.covered = coverage
	s.name = name
	return s.profile, nil
}

func TestCustomerProfileMapsSections(t *testing.T) {
	repo := &stubCustomerRepo{profile: customermysql.Profile{
		Company: "Acme",
		Status:  "active",
		Address: "",
		Website: "acme.co",
		Found:   true,
	}}
	h := NewCustomerProfile(repo)

	rows, err := h.Run(context.Background(), skeleton.Query{
		Text:     "customer Acme",
		Coverage: []int64{3},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (address skipped)", len(rows))
	}
	if rows[0]["section"] != "company" || rows[0]["detail"] != "Acme" {
		t.Fatalf("row0 = %v", rows[0])
	}
	if rows[2]["section"] != "website" || rows[2]["detail"] != "acme.co" {
		t.Fatalf("row2 = %v", rows[2])
	}
}

func TestCustomerProfileNotFound(t *testing.T) {
	repo := &stubCustomerRepo{profile: customermysql.Profile{Found: false}}
	h := NewCustomerProfile(repo)

	rows, err := h.Run(context.Background(), skeleton.Query{Text: "customer Nobody", Coverage: []int64{3}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0 when not found", len(rows))
	}
}

func TestCustomerProfileEmptyCoverage(t *testing.T) {
	repo := &stubCustomerRepo{profile: customermysql.Profile{Company: "Acme", Found: true}}
	h := NewCustomerProfile(repo)

	rows, err := h.Run(context.Background(), skeleton.Query{Text: "Acme", Coverage: nil})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0 on empty coverage", len(rows))
	}
	if repo.covered != nil {
		t.Fatal("repo must not be queried on empty coverage")
	}
}
