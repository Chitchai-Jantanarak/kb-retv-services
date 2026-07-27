package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/my/app/internal/domain/ports"
	apperr "github.com/my/app/internal/shared/errors"
)

func TestTranscribeSendsInlineAudioAndReturnsText(t *testing.T) {
	var captured transcribeRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "models/gemini-2.5-flash:generateContent") {
			t.Fatalf("path = %s, want gemini-2.5-flash", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_ = json.NewEncoder(w).Encode(transcribeResponse{
			Candidates: []struct {
				Content transcribeContent `json:"content"`
			}{
				{Content: transcribeContent{Parts: []transcribePart{{Text: "hello world"}}}},
			},
		})
	}))
	defer srv.Close()

	c := New("gk", "", srv.URL, 0)
	out, err := c.Transcribe(context.Background(), ports.AudioInput{
		Bytes:      []byte("fake-audio-bytes"),
		MIMEType:   "audio/webm",
		LocaleHint: "th",
	})
	if err != nil {
		t.Fatalf("Transcribe() err = %v", err)
	}
	if out.Text != "hello world" {
		t.Fatalf("Text = %q, want hello world", out.Text)
	}
	if len(captured.Contents) != 1 || len(captured.Contents[0].Parts) != 2 {
		t.Fatalf("captured contents = %+v, want 1 content with 2 parts", captured.Contents)
	}
	audioPart := captured.Contents[0].Parts[1]
	if audioPart.InlineData == nil || audioPart.InlineData.MIMEType != "audio/webm" {
		t.Fatalf("inlineData = %+v, want audio/webm mime type", audioPart.InlineData)
	}
	if !strings.Contains(captured.Contents[0].Parts[0].Text, "th") {
		t.Fatalf("instruction text = %q, want locale hint th", captured.Contents[0].Parts[0].Text)
	}
}

func TestTranscribeReturnsErrorOn429And500(t *testing.T) {
	cases := []int{http.StatusTooManyRequests, http.StatusInternalServerError}
	for _, status := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
		}))
		c := New("gk", "", srv.URL, 0)
		_, err := c.Transcribe(context.Background(), ports.AudioInput{Bytes: []byte("audio"), MIMEType: "audio/webm"})
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: Transcribe() err = nil, want error", status)
		}
		appErr, ok := apperr.As(err)
		if !ok || appErr.Code != apperr.CodeUpstreamFailed {
			t.Fatalf("status %d: err = %v, want upstream_failed code", status, err)
		}
	}
}

func TestTranscribeReturnsErrorOnEmptyCandidates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(transcribeResponse{})
	}))
	defer srv.Close()

	c := New("gk", "", srv.URL, 0)
	_, err := c.Transcribe(context.Background(), ports.AudioInput{Bytes: []byte("audio"), MIMEType: "audio/webm"})
	if err == nil {
		t.Fatal("Transcribe() err = nil, want error on empty candidates")
	}
}

func TestTranscribeRejectsEmptyAudioBytes(t *testing.T) {
	c := New("gk", "", "http://unused.local", 0)
	_, err := c.Transcribe(context.Background(), ports.AudioInput{MIMEType: "audio/webm"})
	if err == nil {
		t.Fatal("Transcribe() err = nil, want error on empty bytes")
	}
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeInvalidInput {
		t.Fatalf("err = %v, want invalid_input code", err)
	}
}
