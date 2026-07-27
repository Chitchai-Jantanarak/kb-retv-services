package ports

import "context"

type AudioInput struct {
	Bytes      []byte
	MIMEType   string
	LocaleHint string
}

type Transcript struct {
	Text string
}

type Transcriber interface {
	Transcribe(ctx context.Context, in AudioInput) (Transcript, error)
}
