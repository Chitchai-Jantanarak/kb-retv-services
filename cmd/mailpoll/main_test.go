package main

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"testing"
)

func buildRawEmail(t *testing.T, attachmentSizes []int) []byte {
	t.Helper()

	var multipartBody bytes.Buffer
	mw := multipart.NewWriter(&multipartBody)

	textHeader := make(textproto.MIMEHeader)
	textHeader.Set("Content-Type", "text/plain; charset=utf-8")
	textPart, err := mw.CreatePart(textHeader)
	if err != nil {
		t.Fatalf("create text part: %v", err)
	}
	if _, err := textPart.Write([]byte("hello from the customer")); err != nil {
		t.Fatalf("write text part: %v", err)
	}

	for i, size := range attachmentSizes {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Type", "application/octet-stream")
		header.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="file-%d.bin"`, i))
		part, err := mw.CreatePart(header)
		if err != nil {
			t.Fatalf("create attachment part %d: %v", i, err)
		}
		if _, err := part.Write(bytes.Repeat([]byte("A"), size)); err != nil {
			t.Fatalf("write attachment part %d: %v", i, err)
		}
	}

	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: sender@example.com\r\n")
	fmt.Fprintf(&msg, "To: dest@example.com\r\n")
	fmt.Fprintf(&msg, "Subject: test\r\n")
	fmt.Fprintf(&msg, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "List-Unsubscribe: <mailto:unsub@example.com>\r\n")
	fmt.Fprintf(&msg, "Auto-Submitted: auto-replied\r\n")
	fmt.Fprintf(&msg, "Precedence: bulk\r\n")
	fmt.Fprintf(&msg, "Content-Type: multipart/mixed; boundary=%s\r\n", mw.Boundary())
	msg.WriteString("\r\n")
	msg.Write(multipartBody.Bytes())
	return msg.Bytes()
}

func TestParseMessageAttachmentCap(t *testing.T) {
	orig := maxAttachmentBytes
	maxAttachmentBytes = 10
	defer func() { maxAttachmentBytes = orig }()

	raw := buildRawEmail(t, []int{6, 6, 6})
	result := parseMessage(raw)

	if got := len(result.Attachments); got != 1 {
		t.Fatalf("expected 1 attachment to survive the cap, got %d: %+v", got, result.Attachments)
	}
	if result.Attachments[0].SizeBytes != 6 {
		t.Fatalf("expected first attachment size 6, got %d", result.Attachments[0].SizeBytes)
	}

	total := 0
	for _, a := range result.Attachments {
		total += a.SizeBytes
	}
	if total > maxAttachmentBytes {
		t.Fatalf("total attachment bytes %d exceeded cap %d", total, maxAttachmentBytes)
	}
}

func TestParseMessageAttachmentsUnderCap(t *testing.T) {
	orig := maxAttachmentBytes
	maxAttachmentBytes = 100
	defer func() { maxAttachmentBytes = orig }()

	raw := buildRawEmail(t, []int{10, 20})
	result := parseMessage(raw)

	if got := len(result.Attachments); got != 2 {
		t.Fatalf("expected 2 attachments, got %d: %+v", got, result.Attachments)
	}
	if result.Text != "hello from the customer" {
		t.Fatalf("unexpected text body: %q", result.Text)
	}
	if !result.ListUnsubscribe {
		t.Fatal("expected ListUnsubscribe to be true")
	}
	if result.AutoSubmitted != "auto-replied" {
		t.Fatalf("unexpected AutoSubmitted: %q", result.AutoSubmitted)
	}
	if result.Precedence != "bulk" {
		t.Fatalf("unexpected Precedence: %q", result.Precedence)
	}
}
