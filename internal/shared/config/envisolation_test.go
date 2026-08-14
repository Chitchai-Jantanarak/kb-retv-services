package config

import (
	"os"
	"strings"
	"testing"
)

func isolateEnv(t *testing.T) {
	t.Helper()
	// NOTE: primes testing's once-per-binary TempDir root before env is cleared, else it caches the no-perm Windows fallback dir.
	t.TempDir()
	saved := os.Environ()
	os.Clearenv()
	t.Cleanup(func() {
		os.Clearenv()
		for _, kv := range saved {
			if i := strings.IndexByte(kv, '='); i > 0 {
				os.Setenv(kv[:i], kv[i+1:])
			}
		}
	})
}

func TestIsolateEnvRestoresProcessEnvironment(t *testing.T) {
	os.Setenv("SJT_ISOLATION_CANARY", "before")
	defer os.Unsetenv("SJT_ISOLATION_CANARY")

	t.Run("inner", func(t *testing.T) {
		isolateEnv(t)
		if got := os.Getenv("SJT_ISOLATION_CANARY"); got != "" {
			t.Fatalf("canary = %q inside isolation, want cleared", got)
		}
	})

	if got := os.Getenv("SJT_ISOLATION_CANARY"); got != "before" {
		t.Fatalf("canary = %q after isolation, want restored", got)
	}
}
