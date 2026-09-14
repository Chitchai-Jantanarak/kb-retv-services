package gemini

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/my/app/internal/domain/ports"
)

// Live probe: a real streamGenerateContent call must yield a usage chunk.
func TestProbeStreamUsageLive(t *testing.T) {
	if os.Getenv("PROBE") == "" {
		t.Skip("set PROBE=1")
	}
	key := os.Getenv("APIKEYS_GEMINI")
	if key == "" {
		t.Skip("no gemini key")
	}
	c, err := New(Config{APIKey: key, Model: "gemini-2.5-flash"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ch, err := c.Stream(ctx, ports.Prompt{User: "Reply with the single word: pong"})
	if err != nil {
		t.Fatal(err)
	}
	chunks, text := 0, ""
	var usage ports.TokenUsage
	for chunk := range ch {
		chunks++
		text += chunk.Text
		if chunk.Usage != (ports.TokenUsage{}) {
			usage = chunk.Usage
		}
	}
	t.Logf("chunks=%d text=%q usage=%+v", chunks, text, usage)
	if usage.Input == 0 || usage.Total == 0 {
		t.Fatalf("streamed call recorded no usage: %+v", usage)
	}
}
