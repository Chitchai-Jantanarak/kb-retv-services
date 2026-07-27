package ports

import "testing"

func TestPromptCarriesAttachments(t *testing.T) {
	p := Prompt{
		Attachments: []Attachment{
			{ID: "att-1", MIMEType: "image/png", StorageKey: "s3://bucket/key.png", URL: "https://cdn.example.com/key.png", SizeBytes: 1024},
		},
	}
	if len(p.Attachments) != 1 {
		t.Fatalf("Attachments = %+v, want 1 entry", p.Attachments)
	}
	if p.Attachments[0].ID != "att-1" {
		t.Fatalf("Attachments[0].ID = %q, want att-1", p.Attachments[0].ID)
	}
}
