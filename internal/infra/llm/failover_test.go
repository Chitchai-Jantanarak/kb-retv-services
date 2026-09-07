package llm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm/llmerr"
)

type fakeProvider struct {
	calls int
	err   error
	out   ports.Completion
}

func (f *fakeProvider) Generate(_ context.Context, _ ports.Prompt) (ports.Completion, error) {
	f.calls++
	if f.err != nil {
		return ports.Completion{}, f.err
	}
	return f.out, nil
}

func (f *fakeProvider) GenerateJSON(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	return f.Generate(ctx, p)
}

func (f *fakeProvider) Stream(ctx context.Context, p ports.Prompt) (<-chan ports.Completion, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	ch := make(chan ports.Completion, 1)
	ch <- f.out
	close(ch)
	return ch, nil
}

func TestFailoverModeErrorUsesBackupOnPrimaryFailure(t *testing.T) {
	primary := &fakeProvider{err: errors.New("boom")}
	backup := &fakeProvider{out: ports.Completion{Text: "backup-ok"}}
	f := NewFailover(primary, backup, FailoverError)

	out, err := f.Generate(context.Background(), ports.Prompt{})
	if err != nil {
		t.Fatalf("Generate err = %v, want nil (backup should succeed)", err)
	}
	if out.Text != "backup-ok" {
		t.Fatalf("out.Text = %q, want backup-ok", out.Text)
	}
	if primary.calls != 1 || backup.calls != 1 {
		t.Fatalf("primary.calls = %d, backup.calls = %d, want 1 and 1", primary.calls, backup.calls)
	}
}

func TestFailoverModeOffNeverCallsBackup(t *testing.T) {
	primary := &fakeProvider{err: errors.New("boom")}
	backup := &fakeProvider{out: ports.Completion{Text: "backup-ok"}}
	f := NewFailover(primary, backup, FailoverOff)

	_, err := f.Generate(context.Background(), ports.Prompt{})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want primary error to bubble up", err)
	}
	if backup.calls != 0 {
		t.Fatalf("backup.calls = %d, want 0", backup.calls)
	}
}

func TestFailoverModeQuotaSkipsBackupOnNonQuotaError(t *testing.T) {
	primary := &fakeProvider{err: errors.New("bad request: invalid prompt")}
	backup := &fakeProvider{out: ports.Completion{Text: "backup-ok"}}
	f := NewFailover(primary, backup, FailoverQuota)

	_, err := f.Generate(context.Background(), ports.Prompt{})
	if err == nil || !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("err = %v, want primary error to bubble up", err)
	}
	if backup.calls != 0 {
		t.Fatalf("backup.calls = %d, want 0 (non-quota error must not fail over)", backup.calls)
	}
}

func TestFailoverModeQuotaUsesBackupOnQuotaError(t *testing.T) {
	primary := &fakeProvider{err: &llmerr.ProviderError{Vendor: "openai", Status: 429, Message: "rate limited"}}
	backup := &fakeProvider{out: ports.Completion{Text: "backup-ok"}}
	f := NewFailover(primary, backup, FailoverQuota)

	out, err := f.Generate(context.Background(), ports.Prompt{})
	if err != nil {
		t.Fatalf("Generate err = %v, want nil (backup should succeed)", err)
	}
	if out.Text != "backup-ok" {
		t.Fatalf("out.Text = %q, want backup-ok", out.Text)
	}
	if backup.calls != 1 {
		t.Fatalf("backup.calls = %d, want 1", backup.calls)
	}
}

func TestFailoverModeQuotaUsesBackupOnCircuitOpen(t *testing.T) {
	primary := &fakeProvider{err: ErrCircuitOpen}
	backup := &fakeProvider{out: ports.Completion{Text: "backup-ok"}}
	f := NewFailover(primary, backup, FailoverQuota)

	_, err := f.Generate(context.Background(), ports.Prompt{})
	if err != nil {
		t.Fatalf("Generate err = %v, want nil (backup should succeed)", err)
	}
	if backup.calls != 1 {
		t.Fatalf("backup.calls = %d, want 1", backup.calls)
	}
}

func TestFailoverBothFailReturnsWrappedError(t *testing.T) {
	primary := &fakeProvider{err: errors.New("primary down")}
	backup := &fakeProvider{err: errors.New("backup down")}
	f := NewFailover(primary, backup, FailoverError)

	_, err := f.Generate(context.Background(), ports.Prompt{})
	if err == nil {
		t.Fatal("err = nil, want error mentioning both primary and backup")
	}
	if !strings.Contains(err.Error(), "primary down") || !strings.Contains(err.Error(), "backup down") {
		t.Fatalf("err = %q, want it to mention both failures", err.Error())
	}
}

func TestFailoverGenerateJSONAndStreamDelegate(t *testing.T) {
	primary := &fakeProvider{err: errors.New("boom")}
	backup := &fakeProvider{out: ports.Completion{Text: "backup-ok"}}
	f := NewFailover(primary, backup, FailoverError)

	jsonOut, err := f.GenerateJSON(context.Background(), ports.Prompt{})
	if err != nil || jsonOut.Text != "backup-ok" {
		t.Fatalf("GenerateJSON out = %+v, err = %v", jsonOut, err)
	}

	primary2 := &fakeProvider{err: errors.New("boom")}
	backup2 := &fakeProvider{out: ports.Completion{Text: "stream-ok"}}
	f2 := NewFailover(primary2, backup2, FailoverError)

	ch, err := f2.Stream(context.Background(), ports.Prompt{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}
	var got []string
	for c := range ch {
		got = append(got, c.Text)
	}
	if strings.Join(got, "") != "stream-ok" {
		t.Fatalf("stream chunks = %v, want stream-ok", got)
	}
}

func TestNormalizeFailoverModeInvalidBecomesOff(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", FailoverOff},
		{"OFF", FailoverOff},
		{"bogus", FailoverOff},
		{" Error ", FailoverError},
		{"QUOTA", FailoverQuota},
	}
	for _, tc := range cases {
		if got := normalizeFailoverMode(tc.in); got != tc.want {
			t.Errorf("normalizeFailoverMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
