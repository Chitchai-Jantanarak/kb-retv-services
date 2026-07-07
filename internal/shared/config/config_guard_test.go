package config

import "testing"

func TestChatHasGuardEmbedderFields(t *testing.T) {
	t.Parallel()
	c := Chat{GuardEmbedderProvider: "static-local", GuardEmbedderAssetDir: "/assets/g"}
	if c.GuardEmbedderProvider != "static-local" || c.GuardEmbedderAssetDir != "/assets/g" {
		t.Fatalf("guard embedder fields not wired: %+v", c)
	}
}
