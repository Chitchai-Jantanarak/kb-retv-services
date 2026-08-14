package chat

import (
	"context"
	"testing"

	"github.com/my/app/internal/application/dto"
)

func TestReplyPoolPickEmptyReturnsFalse(t *testing.T) {
	pool := newReplyPool(nil)
	if _, ok := pool.pick("social", "greeting", dto.ChatLocaleEnglish, 0); ok {
		t.Fatal("pick() ok = true, want false for empty pool")
	}
}

func TestReplyPoolPickNilPoolReturnsFalse(t *testing.T) {
	var pool *replyPool
	if _, ok := pool.pick("social", "greeting", dto.ChatLocaleEnglish, 0); ok {
		t.Fatal("pick() ok = true, want false for nil pool")
	}
}

func TestReplyPoolPickDeterministicForSameSeed(t *testing.T) {
	pool := newReplyPool(nil)
	pool.entries[poolKey("social", "greeting", dto.ChatLocaleEnglish)] = []string{"a", "b", "c", "d", "e", "f"}

	first, ok := pool.pick("social", "greeting", dto.ChatLocaleEnglish, 42)
	if !ok {
		t.Fatal("pick() ok = false, want true")
	}
	second, ok := pool.pick("social", "greeting", dto.ChatLocaleEnglish, 42)
	if !ok {
		t.Fatal("pick() ok = false, want true")
	}
	if first != second {
		t.Fatalf("pick() = %q then %q, want same reply for same seed", first, second)
	}
}

func TestReplyPoolWarmPopulatesFromStubProvider(t *testing.T) {
	provider := &stubProvider{text: `{"variants":["a","b","c","d","e","f"]}`}
	pool := newReplyPool(provider)

	pool.warm(context.Background())

	reply, ok := pool.pick("social", "greeting", dto.ChatLocaleEnglish, 0)
	if !ok {
		t.Fatal("pick() ok = false after warm, want true")
	}
	if reply == "" {
		t.Fatal("pick() returned empty reply after warm")
	}

	if _, ok := pool.pick("handoff", "", dto.ChatLocaleThai, 0); !ok {
		t.Fatal("pick() ok = false for handoff/th after warm, want true")
	}

	if provider.calls == 0 {
		t.Fatal("warm() made no GenerateJSON calls")
	}
}

func TestReplyPoolWarmInertWhenProviderNil(t *testing.T) {
	pool := newReplyPool(nil)
	pool.warm(context.Background())

	if _, ok := pool.pick("social", "greeting", dto.ChatLocaleEnglish, 0); ok {
		t.Fatal("pick() ok = true after warm with nil provider, want false")
	}
}

func TestReplyPoolWarmSkipsOnBadJSON(t *testing.T) {
	provider := &stubProvider{text: "not json"}
	pool := newReplyPool(provider)

	pool.warm(context.Background())

	if _, ok := pool.pick("social", "greeting", dto.ChatLocaleEnglish, 0); ok {
		t.Fatal("pick() ok = true after warm with bad JSON, want false")
	}
}
