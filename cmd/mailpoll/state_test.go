package main

import (
	"path/filepath"
	"testing"
)

func TestUIDStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "state.json")
	if st := loadState(path); st.LastUID != 0 || st.UIDValidity != 0 {
		t.Fatalf("missing file must yield zero state, got %+v", st)
	}
	want := uidState{UIDValidity: 42, LastUID: 918}
	saveState(path, want)
	if got := loadState(path); got != want {
		t.Fatalf("round trip: got %+v want %+v", got, want)
	}
}
