package tenant

import (
	"errors"
	"testing"
)

func TestErrRowSurfacesRoutingError(t *testing.T) {
	want := errors.New("company_id missing from context")
	var out int
	err := errRow(want).Scan(&out)
	if err == nil || !errors.Is(err, want) {
		t.Fatalf("Scan err = %v, want wrapping %v", err, want)
	}
}
