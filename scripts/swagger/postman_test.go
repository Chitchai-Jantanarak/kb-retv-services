package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPostmanReviewCallbackScriptsUseTimestampedHMAC(t *testing.T) {
	specBytes, err := os.ReadFile("../../docs/postman/onlinesupport-go-ai.postman_collection.json")
	if err != nil {
		t.Fatalf("read postman collection: %v", err)
	}

	var collection map[string]any
	if err := json.Unmarshal(specBytes, &collection); err != nil {
		t.Fatalf("parse postman collection: %v", err)
	}

	callbacks := postmanFolder(t, collection, "Laravel Callback")
	if len(callbacks) != 3 {
		t.Fatalf("Laravel Callback request count = %d, want 3", len(callbacks))
	}

	for _, name := range []string{
		"POST review queue callback - valid signed",
		"POST review queue callback - idempotent replay",
		"POST review queue callback - stale signature rejected",
	} {
		item := postmanItem(t, callbacks, name)
		script := postmanScript(t, item, "prerequest")
		for _, want := range []string{
			"CryptoJS.HmacSHA256",
			"timestamp + \".\" + raw",
			"X-AI-Timestamp",
			"X-AI-Signature",
		} {
			if !containsSubstring(script, want) {
				t.Fatalf("%s prerequest script missing %q", name, want)
			}
		}
	}
}

func TestPostmanEnvironmentDocumentsCallbackVariables(t *testing.T) {
	specBytes, err := os.ReadFile("../../docs/postman/onlinesupport-go-ai.postman_environment.json")
	if err != nil {
		t.Fatalf("read postman environment: %v", err)
	}

	var environment map[string]any
	if err := json.Unmarshal(specBytes, &environment); err != nil {
		t.Fatalf("parse postman environment: %v", err)
	}

	for _, key := range []string{
		"laravel_base_url",
		"ai_webhook_secret",
		"ai_webhook_tolerance_seconds",
		"callback_company_id",
		"review_payload_hash",
		"review_callback_name",
		"review_callback_item_id",
	} {
		if !postmanEnvironmentHasKey(t, environment, key) {
			t.Fatalf("postman environment missing %s", key)
		}
	}
}

func TestPostmanCollectionIncludesRAGFoundationDebugSet(t *testing.T) {
	specBytes, err := os.ReadFile("../../docs/postman/onlinesupport-go-ai.postman_collection.json")
	if err != nil {
		t.Fatalf("read postman collection: %v", err)
	}

	var collection map[string]any
	if err := json.Unmarshal(specBytes, &collection); err != nil {
		t.Fatalf("parse postman collection: %v", err)
	}

	rag := postmanFolder(t, collection, "RAG Foundation Debug")
	for _, name := range []string{
		"POST reply - debug trace baseline",
		"POST reply - graph anchor confirmed symptom",
		"POST reply - AI symptom confidence gate",
		"POST reply - low confidence graph skipped",
		"POST reply - parent context expansion smoke",
	} {
		item := postmanItem(t, rag, name)
		script := postmanScript(t, item, "test")
		if !containsSubstring(script, "debug_trace") {
			t.Fatalf("%s test script missing debug_trace assertion", name)
		}
	}
}

func TestPostmanCollectionIncludesChatDebugTimingSet(t *testing.T) {
	specBytes, err := os.ReadFile("../../docs/postman/onlinesupport-go-ai.postman_collection.json")
	if err != nil {
		t.Fatalf("read postman collection: %v", err)
	}

	var collection map[string]any
	if err := json.Unmarshal(specBytes, &collection); err != nil {
		t.Fatalf("parse postman collection: %v", err)
	}

	chat := postmanFolder(t, collection, "Chat Debug Timing")
	tests := map[string][]string{
		"POST chat - fast guard timing": {
			"stage_timings_ms",
			"fast_guard",
			"generate",
		},
		"POST chat - handoff router timing": {
			"stage_timings_ms",
			"router",
			"generate",
		},
		"POST chat - KB cold timing": {
			"stage_timings_ms",
			"generate",
			"knowledge",
		},
		"POST chat - KB repeat cache timing": {
			"stage_timings_ms",
			"cache",
			"generate",
		},
	}
	for name, wants := range tests {
		item := postmanItem(t, chat, name)
		script := postmanScript(t, item, "test")
		for _, want := range wants {
			if !containsSubstring(script, want) {
				t.Fatalf("%s test script missing %q", name, want)
			}
		}
	}
}

func TestPostmanEnvironmentDocumentsRAGFoundationVariables(t *testing.T) {
	specBytes, err := os.ReadFile("../../docs/postman/onlinesupport-go-ai.postman_environment.json")
	if err != nil {
		t.Fatalf("read postman environment: %v", err)
	}

	var environment map[string]any
	if err := json.Unmarshal(specBytes, &environment); err != nil {
		t.Fatalf("parse postman environment: %v", err)
	}

	for _, key := range []string{
		"symptom_node_id",
		"ai_symptom_node_id",
		"parent_context_issue",
		"chat_known_issue",
		"chat_off_domain_probe",
	} {
		if !postmanEnvironmentHasKey(t, environment, key) {
			t.Fatalf("postman environment missing %s", key)
		}
	}
}

func postmanFolder(t *testing.T, collection map[string]any, name string) []any {
	t.Helper()

	items, ok := collection["item"].([]any)
	if !ok {
		t.Fatalf("collection items missing or invalid")
	}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if item["name"] == name {
			children, ok := item["item"].([]any)
			if !ok {
				t.Fatalf("folder %s children missing or invalid", name)
			}
			return children
		}
	}
	t.Fatalf("folder %s missing", name)
	return nil
}

func postmanItem(t *testing.T, items []any, name string) map[string]any {
	t.Helper()

	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if ok && item["name"] == name {
			return item
		}
	}
	t.Fatalf("postman item %s missing", name)
	return nil
}

func postmanScript(t *testing.T, item map[string]any, listen string) string {
	t.Helper()

	events, ok := item["event"].([]any)
	if !ok {
		t.Fatalf("item events missing or invalid")
	}
	for _, raw := range events {
		event, ok := raw.(map[string]any)
		if !ok || event["listen"] != listen {
			continue
		}
		script, ok := event["script"].(map[string]any)
		if !ok {
			t.Fatalf("%s script missing or invalid", listen)
		}
		lines, ok := script["exec"].([]any)
		if !ok {
			t.Fatalf("%s script exec missing or invalid", listen)
		}
		var out string
		for _, line := range lines {
			text, ok := line.(string)
			if !ok {
				t.Fatalf("%s script contains non-string line", listen)
			}
			out += text + "\n"
		}
		return out
	}
	t.Fatalf("%s event missing", listen)
	return ""
}

func postmanEnvironmentHasKey(t *testing.T, environment map[string]any, key string) bool {
	t.Helper()

	values, ok := environment["values"].([]any)
	if !ok {
		t.Fatalf("environment values missing or invalid")
	}
	for _, raw := range values {
		value, ok := raw.(map[string]any)
		if ok && value["key"] == key {
			return true
		}
	}
	return false
}
