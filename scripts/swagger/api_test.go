package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestGeneratedSwaggerUsesStructuredJSONRequestBodies(t *testing.T) {
	specBytes, err := os.ReadFile("../../docs/swagger/swagger.json")
	if err != nil {
		t.Fatalf("read swagger.json: %v", err)
	}

	var spec map[string]any
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		t.Fatalf("parse swagger.json: %v", err)
	}

	tests := []struct {
		path    string
		bodyRef string
	}{
		{path: "/v1/inbound/line", bodyRef: "#/definitions/scripts_swagger.lineInboundWebhookRequest"},
		{path: "/v1/inbound/email", bodyRef: "#/definitions/scripts_swagger.emailInboundWebhookRequest"},
		{path: "/v1/reply/feedback", bodyRef: "#/definitions/scripts_swagger.feedbackRequest"},
		{path: "/v1/admin/review-queue/{id}/reject", bodyRef: "#/definitions/scripts_swagger.reviewRejectRequest"},
		{path: "/internal/review-queue", bodyRef: "#/definitions/scripts_swagger.reviewQueueCallbackRequest"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			post := swaggerPostOperation(t, spec, tt.path)

			consumes := stringSlice(t, post["consumes"], "consumes")
			if !contains(consumes, "application/json") {
				t.Fatalf("consumes = %v, want application/json", consumes)
			}

			body := bodyParameter(t, post)
			schema, ok := body["schema"].(map[string]any)
			if !ok {
				t.Fatalf("body schema missing or invalid: %#v", body["schema"])
			}
			if got := schema["$ref"]; got != tt.bodyRef {
				t.Fatalf("body schema ref = %v, want %s", got, tt.bodyRef)
			}
			if schema["type"] == "string" || schema["format"] == "binary" {
				t.Fatalf("body schema is raw/binary: %#v", schema)
			}
		})
	}
}

func TestGeneratedSwaggerDocumentsReviewCallbackHMACHeaders(t *testing.T) {
	specBytes, err := os.ReadFile("../../docs/swagger/swagger.json")
	if err != nil {
		t.Fatalf("read swagger.json: %v", err)
	}

	var spec map[string]any
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		t.Fatalf("parse swagger.json: %v", err)
	}

	post := swaggerPostOperation(t, spec, "/internal/review-queue")
	timestamp := namedParameter(t, post, "header", "X-AI-Timestamp")
	if timestamp["required"] != true {
		t.Fatalf("X-AI-Timestamp required = %v, want true", timestamp["required"])
	}

	signature := namedParameter(t, post, "header", "X-AI-Signature")
	desc, _ := signature["description"].(string)
	if !containsSubstring(desc, "HMAC-SHA256") || !containsSubstring(desc, "<timestamp>.<raw body>") {
		t.Fatalf("X-AI-Signature description = %q, want timestamped HMAC contract", desc)
	}

	responses, ok := post["responses"].(map[string]any)
	if !ok {
		t.Fatalf("responses missing or invalid")
	}
	if _, ok := responses["201"]; !ok {
		t.Fatalf("201 response missing")
	}
	if _, ok := responses["200"]; !ok {
		t.Fatalf("200 idempotent response missing")
	}
}

func swaggerPostOperation(t *testing.T, spec map[string]any, path string) map[string]any {
	t.Helper()

	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths missing or invalid")
	}
	pathItem, ok := paths[path].(map[string]any)
	if !ok {
		t.Fatalf("path %s missing", path)
	}
	post, ok := pathItem["post"].(map[string]any)
	if !ok {
		t.Fatalf("post operation missing for %s", path)
	}
	return post
}

func bodyParameter(t *testing.T, post map[string]any) map[string]any {
	t.Helper()

	params, ok := post["parameters"].([]any)
	if !ok {
		t.Fatalf("parameters missing or invalid")
	}
	for _, param := range params {
		p, ok := param.(map[string]any)
		if ok && p["in"] == "body" {
			return p
		}
	}
	t.Fatalf("body parameter missing")
	return nil
}

func namedParameter(t *testing.T, post map[string]any, in string, name string) map[string]any {
	t.Helper()

	params, ok := post["parameters"].([]any)
	if !ok {
		t.Fatalf("parameters missing or invalid")
	}
	for _, param := range params {
		p, ok := param.(map[string]any)
		if ok && p["in"] == in && p["name"] == name {
			return p
		}
	}
	t.Fatalf("%s parameter %s missing", in, name)
	return nil
}

func stringSlice(t *testing.T, value any, field string) []string {
	t.Helper()

	values, ok := value.([]any)
	if !ok {
		t.Fatalf("%s missing or invalid: %#v", field, value)
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		s, ok := value.(string)
		if !ok {
			t.Fatalf("%s contains non-string value: %#v", field, value)
		}
		out = append(out, s)
	}
	return out
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsSubstring(value string, want string) bool {
	for i := 0; i+len(want) <= len(value); i++ {
		if value[i:i+len(want)] == want {
			return true
		}
	}
	return false
}
