package ctxkey

import (
	"context"
	"testing"
)

func TestCompanyIDRoundTrip(t *testing.T) {
	ctx := WithCompanyID(context.Background(), 42)

	got, ok := CompanyID(ctx)
	if !ok {
		t.Fatal("CompanyID() ok = false, want true")
	}
	if got != 42 {
		t.Fatalf("CompanyID() = %d, want 42", got)
	}
	if MustCompanyID(ctx) != 42 {
		t.Fatalf("MustCompanyID() = %d, want 42", MustCompanyID(ctx))
	}
}

func TestCompanyIDMissing(t *testing.T) {
	if got, ok := CompanyID(context.Background()); ok || got != 0 {
		t.Fatalf("CompanyID() = %d, %v; want 0, false", got, ok)
	}
}

func TestMustCompanyIDPanicsWhenMissing(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustCompanyID() did not panic")
		}
	}()

	MustCompanyID(context.Background())
}
