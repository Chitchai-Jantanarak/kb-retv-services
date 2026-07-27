package ports

import "context"

type AttachmentData struct {
	Bytes    []byte
	MIMEType string
}

type AttachmentFetcher interface {
	Fetch(ctx context.Context, att Attachment) (AttachmentData, error)
}
