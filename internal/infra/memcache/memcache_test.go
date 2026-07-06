package memcache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/my/app/internal/domain/ports"
)

var _ ports.Cache = (*Cache)(nil)

func TestSetGetRoundTrip(t *testing.T) {
	c := New(8)
	ctx := context.Background()

	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "v" {
		t.Fatalf("Get = %q, want v", got)
	}
}

func TestGetMissAndExpiry(t *testing.T) {
	c := New(8)
	ctx := context.Background()

	if _, err := c.Get(ctx, "absent"); !errors.Is(err, ErrMiss) {
		t.Fatalf("Get(absent) err = %v, want ErrMiss", err)
	}

	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	c.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if _, err := c.Get(ctx, "k"); !errors.Is(err, ErrMiss) {
		t.Fatalf("Get(expired) err = %v, want ErrMiss", err)
	}
}

func TestSetEvictsWhenFull(t *testing.T) {
	c := New(2)
	ctx := context.Background()

	if err := c.Set(ctx, "a", []byte("1"), time.Minute); err != nil {
		t.Fatalf("Set a: %v", err)
	}
	if err := c.Set(ctx, "b", []byte("2"), time.Minute); err != nil {
		t.Fatalf("Set b: %v", err)
	}
	if err := c.Set(ctx, "c", []byte("3"), time.Minute); err != nil {
		t.Fatalf("Set c: %v", err)
	}
	if len(c.items) > 2 {
		t.Fatalf("len(items) = %d, want <= 2", len(c.items))
	}
	if _, err := c.Get(ctx, "c"); err != nil {
		t.Fatalf("newest entry evicted: %v", err)
	}
}

func TestDeleteAndSetNX(t *testing.T) {
	c := New(8)
	ctx := context.Background()

	ok, err := c.SetNX(ctx, "k", []byte("v1"), time.Minute)
	if err != nil || !ok {
		t.Fatalf("SetNX first = (%v, %v), want (true, nil)", ok, err)
	}
	ok, err = c.SetNX(ctx, "k", []byte("v2"), time.Minute)
	if err != nil || ok {
		t.Fatalf("SetNX second = (%v, %v), want (false, nil)", ok, err)
	}
	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := c.Get(ctx, "k"); !errors.Is(err, ErrMiss) {
		t.Fatalf("Get after Delete err = %v, want ErrMiss", err)
	}
}
