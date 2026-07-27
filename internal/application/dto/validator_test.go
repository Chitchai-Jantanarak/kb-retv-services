package dto

import (
	"errors"
	"testing"

	apperr "github.com/my/app/internal/shared/errors"
)

func TestReplyRequestValidateNormalizesAndAcceptsChannelContext(t *testing.T) {
	req := ReplyRequest{
		CustomerID:     " customer-demo ",
		SiteID:         " site-demo ",
		Message:        " printer offline ",
		Channel:        "line",
		ConversationID: " conv-1 ",
		MessageID:      " msg-1 ",
		Attachments: []AttachmentRef{
			{
				URL:      "https://cdn.example.test/files/a.png",
				MIMEType: "image/png",
			},
		},
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if req.Message != "printer offline" {
		t.Fatalf("Message = %q, want printer offline", req.Message)
	}
	if req.CustomerID != "customer-demo" {
		t.Fatalf("CustomerID = %q, want customer-demo", req.CustomerID)
	}
}

func TestReplyRequestValidateRejectsMissingMessage(t *testing.T) {
	req := ReplyRequest{CustomerID: "customer-demo"}

	err := req.Validate()
	if !hasCode(err, apperr.CodeInvalidInput) {
		t.Fatalf("Validate() error = %v, want invalid_input", err)
	}
}

func TestAttachmentRefValidateRejectsBase64DataURL(t *testing.T) {
	req := ReplyRequest{
		CustomerID: "customer-demo",
		Message:    "see image",
		Attachments: []AttachmentRef{
			{URL: "data:image/png;base64,AAAA"},
		},
	}

	err := req.Validate()
	if !hasCode(err, apperr.CodeInvalidInput) {
		t.Fatalf("Validate() error = %v, want invalid_input", err)
	}
}

func TestInboundMessageRequestValidateAllowsBodyOrAttachment(t *testing.T) {
	req := InboundMessageRequest{
		Channel:           "email",
		ExternalMessageID: "mail-1",
		CustomerID:        "customer-demo",
		Attachments: []AttachmentRef{
			{StorageKey: "s3://bucket/key.png", MIMEType: "image/png"},
		},
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestInboundMessageRequestValidateRequiresBodyOrAttachment(t *testing.T) {
	req := InboundMessageRequest{
		Channel:           "line",
		ExternalMessageID: "line-1",
	}

	err := req.Validate()
	if !hasCode(err, apperr.CodeInvalidInput) {
		t.Fatalf("Validate() error = %v, want invalid_input", err)
	}
}

func TestRAGQueryRequestValidateDefaultsLimit(t *testing.T) {
	req := RAGQueryRequest{
		Query: "printer offline",
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if req.Limit != 8 {
		t.Fatalf("Limit = %d, want 8", req.Limit)
	}
}

func TestRAGQueryRequestValidateClampsLimit(t *testing.T) {
	req := RAGQueryRequest{
		Query: "printer offline",
		Limit: 200,
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if req.Limit != 50 {
		t.Fatalf("Limit = %d, want 50", req.Limit)
	}
}

func TestAttachmentRefValidateAcceptsRootRelativeSignedURL(t *testing.T) {
	req := ReplyRequest{
		CustomerID: "customer-demo",
		Message:    "see image",
		Attachments: []AttachmentRef{
			{URL: "/api/ai/attachments/fetch?key=x&signature=y"},
		},
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestAttachmentRefValidateAcceptsAbsoluteHTTPSURL(t *testing.T) {
	req := ReplyRequest{
		CustomerID: "customer-demo",
		Message:    "see image",
		Attachments: []AttachmentRef{
			{URL: "https://app.local/x"},
		},
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestAttachmentRefValidateRejectsJavascriptScheme(t *testing.T) {
	req := ReplyRequest{
		CustomerID: "customer-demo",
		Message:    "see image",
		Attachments: []AttachmentRef{
			{URL: "javascript:alert(1)"},
		},
	}

	err := req.Validate()
	if !hasCode(err, apperr.CodeInvalidInput) {
		t.Fatalf("Validate() error = %v, want invalid_input", err)
	}
}

func TestAttachmentRefValidateRejectsProtocolRelativeURL(t *testing.T) {
	req := ReplyRequest{
		CustomerID: "customer-demo",
		Message:    "see image",
		Attachments: []AttachmentRef{
			{URL: "//evil.com/x"},
		},
	}

	err := req.Validate()
	if !hasCode(err, apperr.CodeInvalidInput) {
		t.Fatalf("Validate() error = %v, want invalid_input", err)
	}
}

func hasCode(err error, code apperr.Code) bool {
	var appErr *apperr.AppError
	return errors.As(err, &appErr) && appErr.Code == code
}
