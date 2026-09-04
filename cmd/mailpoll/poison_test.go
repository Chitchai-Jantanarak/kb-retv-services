package main

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestSkipUIDRetriesThenSkips(t *testing.T) {
	cfg := config{statePath: filepath.Join(t.TempDir(), "state.json"), maxAttempts: 3}
	st := uidState{UIDValidity: 7, LastUID: 100}
	cause := errors.New("inbound returned status 500")

	for attempt := 1; attempt < cfg.maxAttempts; attempt++ {
		if skipUID(&st, cfg, 101, cause) {
			t.Fatalf("attempt %d: skipped too early", attempt)
		}
		if st.LastUID != 100 {
			t.Fatalf("attempt %d: cursor moved early to %d", attempt, st.LastUID)
		}
		if st.FailCount != attempt {
			t.Fatalf("attempt %d: FailCount = %d", attempt, st.FailCount)
		}
	}

	if !skipUID(&st, cfg, 101, cause) {
		t.Fatal("final attempt must skip the poison uid")
	}
	if st.LastUID != 101 {
		t.Fatalf("cursor must advance past skipped uid, got %d", st.LastUID)
	}
	if st.FailUID != 0 || st.FailCount != 0 {
		t.Fatalf("counters must reset after skip, got uid=%d count=%d", st.FailUID, st.FailCount)
	}

	// The skip must survive a restart, or the poller loops forever.
	if got := loadState(cfg.statePath); got.LastUID != 101 {
		t.Fatalf("persisted LastUID = %d, want 101", got.LastUID)
	}
}

func TestSkipUIDResetsCountOnDifferentUID(t *testing.T) {
	cfg := config{statePath: filepath.Join(t.TempDir(), "state.json"), maxAttempts: 3}
	st := uidState{LastUID: 100}
	cause := errors.New("boom")

	skipUID(&st, cfg, 101, cause)
	skipUID(&st, cfg, 101, cause)
	if st.FailCount != 2 {
		t.Fatalf("FailCount = %d, want 2", st.FailCount)
	}

	if skipUID(&st, cfg, 102, cause) {
		t.Fatal("a different uid must start a fresh budget, not inherit one")
	}
	if st.FailUID != 102 || st.FailCount != 1 {
		t.Fatalf("got uid=%d count=%d, want 102/1", st.FailUID, st.FailCount)
	}
}
