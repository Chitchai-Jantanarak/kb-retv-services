package mailpoll

import (
	"encoding/base64"
	"strings"
	"testing"
)

// Phone mail: multipart/related wrapping multipart/alternative plus an
// inline JPEG. The JPEG must become an attachment; the body must stay text.
func TestParseMIMEInlineImageIsAttachmentNotBody(t *testing.T) {
	jpeg := []byte("\xff\xd8\xff\xe0fakejpegbytes")
	raw := strings.Join([]string{
		"From: KAIz A <akalzz1121@gmail.com>",
		"To: athidit@grubgrob.xyz",
		"Subject: hello tenants testing",
		"MIME-Version: 1.0",
		"Content-Type: multipart/related; boundary=\"outer\"",
		"",
		"--outer",
		"Content-Type: multipart/alternative; boundary=\"inner\"",
		"",
		"--inner",
		"Content-Type: text/plain; charset=\"UTF-8\"",
		"",
		"this is a test email. how u do",
		"--inner",
		"Content-Type: text/html; charset=\"UTF-8\"",
		"",
		"<div>this is a test email. how u do</div>",
		"--inner--",
		"--outer",
		"Content-Type: image/jpeg; name=\"IMG_0302.jpg\"",
		"Content-Disposition: inline; filename=\"IMG_0302.jpg\"",
		"Content-Transfer-Encoding: base64",
		"",
		base64.StdEncoding.EncodeToString(jpeg),
		"--outer--",
		"",
	}, "\r\n")

	got := ParseMIME([]byte(raw), 1<<20)

	if got.Text != "this is a test email. how u do" {
		t.Fatalf("Text = %q, want the text/plain part", got.Text)
	}
	if !strings.Contains(got.HTML, "<div>") {
		t.Fatalf("HTML = %q, want the text/html part", got.HTML)
	}
	if len(got.Attachments) != 1 {
		t.Fatalf("Attachments = %d, want 1 (the inline JPEG)", len(got.Attachments))
	}
	a := got.Attachments[0]
	if a.Filename != "IMG_0302.jpg" || a.MIMEType != "image/jpeg" || a.SizeBytes != len(jpeg) {
		t.Fatalf("attachment = %+v", a)
	}
}
