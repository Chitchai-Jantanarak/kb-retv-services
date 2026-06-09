package response

import "testing"

func TestNewErrorPreservesPublicFields(t *testing.T) {
	code := "bad_request"
	err := NewError(&Error{
		CodeResponse: 400,
		Message:      "invalid",
		CodeMsg:      &code,
		Data:         map[string]string{"field": "name"},
	})

	if err.CodeResponse != 400 {
		t.Fatalf("expected code response 400, got %d", err.CodeResponse)
	}
	if err.Message != "invalid" {
		t.Fatalf("expected message invalid, got %q", err.Message)
	}
	if err.CodeMsg == nil || *err.CodeMsg != code {
		t.Fatalf("expected code %q, got %#v", code, err.CodeMsg)
	}
	if err.Data == nil {
		t.Fatal("expected data to be preserved")
	}
	if err.Timestamp.IsZero() {
		t.Fatal("expected timestamp")
	}
}

func TestNewSuccessPreservesPublicFields(t *testing.T) {
	code := "ok"
	success := NewSuccess(&Success{
		CodeResponse: 201,
		Message:      "created",
		CodeMsg:      &code,
		Data:         map[string]string{"id": "1"},
	})

	if success.CodeResponse != 201 {
		t.Fatalf("expected code response 201, got %d", success.CodeResponse)
	}
	if success.Message != "created" {
		t.Fatalf("expected message created, got %q", success.Message)
	}
	if success.CodeMsg == nil || *success.CodeMsg != code {
		t.Fatalf("expected code %q, got %#v", code, success.CodeMsg)
	}
	if success.Data == nil {
		t.Fatal("expected data to be preserved")
	}
	if success.Timestamp.IsZero() {
		t.Fatal("expected timestamp")
	}
}
