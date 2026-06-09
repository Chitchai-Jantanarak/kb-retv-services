package omnichannel

import (
	"strings"
	"testing"
)

func TestLineNormalizerFailureCases(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantSub string
	}{
		{name: "bad_json", raw: "not-json", wantSub: "parse"},
		{name: "no_events", raw: `{"destination":"x","events":[]}`, wantSub: "no events"},
		{name: "no_source", raw: `{"destination":"x","events":[{"message":{"id":"m1","text":"hi"}}]}`, wantSub: "no source"},
		{name: "no_message_id", raw: `{"destination":"x","events":[{"source":{"type":"user","userId":"u1"},"message":{"text":"hi"}}]}`, wantSub: "no message id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LineNormalizer{}.Normalize([]byte(tc.raw))
			if err == nil {
				t.Fatalf("err = nil, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want sub %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestLineNormalizerHappyPath(t *testing.T) {
	raw := `{
		"destination": "Ucompany123",
		"events": [
			{"type":"message","timestamp":1718000000,"source":{"type":"user","userId":"Uabc"},"message":{"id":"msg-1","type":"text","text":"พิมพ์ไม่ออก"}}
		]
	}`
	got, err := LineNormalizer{}.Normalize([]byte(raw))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got.Request.Channel != ChannelLine {
		t.Fatalf("channel = %q", got.Request.Channel)
	}
	if got.ExternalSender != "Uabc" {
		t.Fatalf("sender = %q", got.ExternalSender)
	}
	if got.Request.Body != "พิมพ์ไม่ออก" {
		t.Fatalf("body = %q", got.Request.Body)
	}
}

func TestEmailNormalizerFailureCases(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantSub string
	}{
		{name: "bad_json", raw: "not-json", wantSub: "parse"},
		{name: "no_from", raw: `{"message_id":"m1","subject":"x","body":"hi"}`, wantSub: "From"},
		{name: "no_id", raw: `{"from":"x@example.com","body":"hi"}`, wantSub: "message_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := EmailNormalizer{}.Normalize([]byte(tc.raw))
			if err == nil {
				t.Fatalf("err = nil, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want sub %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestEmailNormalizerExtractsAddress(t *testing.T) {
	raw := `{"message_id":"<m-1@mail>","from":"Alice <alice@example.com>","subject":"help","body":"can't print"}`
	got, err := EmailNormalizer{}.Normalize([]byte(raw))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got.ExternalSender != "alice@example.com" {
		t.Fatalf("sender = %q", got.ExternalSender)
	}
	if got.Request.Subject != "help" {
		t.Fatalf("subject = %q", got.Request.Subject)
	}
}

func TestRegistryRejectsDuplicateChannel(t *testing.T) {
	_, err := NewNormalizerRegistry(LineNormalizer{}, LineNormalizer{})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("err = %v, want duplicate-channel error", err)
	}
}

func TestRegistryGetUnknownChannel(t *testing.T) {
	r, err := NewNormalizerRegistry(LineNormalizer{})
	if err != nil {
		t.Fatalf("NewNormalizerRegistry: %v", err)
	}
	if _, err := r.Get("smoke-signal"); err == nil {
		t.Fatal("err = nil, want unknown-channel error")
	}
}
