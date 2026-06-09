package utils

import "testing"

func TestPtrAndStringPtr(t *testing.T) {
	value := Ptr(10)
	if value == nil || *value != 10 {
		t.Fatalf("expected pointer to 10, got %#v", value)
	}

	text := StringPtr("ok")
	if text == nil || *text != "ok" {
		t.Fatalf("expected pointer to ok, got %#v", text)
	}
}

func TestStringPtrFrom(t *testing.T) {
	got := StringPtrFrom(map[string]interface{}{"count": 12}, "count")
	if got == nil || *got != "12" {
		t.Fatalf("expected string pointer 12, got %#v", got)
	}
}
