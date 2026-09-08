package mailpoll

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"strings"

	"github.com/emersion/go-message/mail"
)

// ParsedMessage is the result of pulling the text/HTML body, attachments and a
// handful of headers out of a raw RFC 822 (MIME) message. It is shared by
// every mail reader (IMAP poll, Gmail API poll, ...) so that a message
// reaching the inbound pipeline through a different transport still produces
// an identical EmailPayload body.
type ParsedMessage struct {
	Text            string
	HTML            string
	Attachments     []Attachment
	AutoSubmitted   string
	ListUnsubscribe bool
	Precedence      string
	// DeliveredTo holds every value of the Delivered-To and X-Forwarded-To
	// headers, lowercased, trimmed and de-duplicated in header order. A
	// forwarded message can carry more than one Delivered-To line (one per
	// hop), and either header may name the routing address (+tag or alias)
	// that the original To/Cc no longer show once a forwarder rewrites them.
	DeliveredTo []string
}

// ParseMIME extracts the body, attachments and a few headers from a raw
// RFC 822 message. maxAttachmentBytes caps the total size of attachments kept
// on the result; once the cap is hit, further attachments are dropped (the
// message itself is never rejected for being too large).
func ParseMIME(raw []byte, maxAttachmentBytes int) ParsedMessage {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return ParsedMessage{Text: strings.TrimSpace(string(raw))}
	}

	result := ParsedMessage{
		AutoSubmitted:   strings.TrimSpace(mr.Header.Get("Auto-Submitted")),
		ListUnsubscribe: strings.TrimSpace(mr.Header.Get("List-Unsubscribe")) != "",
		Precedence:      strings.TrimSpace(mr.Header.Get("Precedence")),
		DeliveredTo:     headerValues(mr.Header, "Delivered-To", "X-Forwarded-To"),
	}

	attachmentBytes := 0
	attachmentsCapped := false
	keep := func(filename, mimeType string, b []byte) {
		if attachmentsCapped {
			return
		}
		if attachmentBytes+len(b) > maxAttachmentBytes {
			attachmentsCapped = true
			return
		}
		attachmentBytes += len(b)
		result.Attachments = append(result.Attachments, Attachment{
			Filename:   filename,
			MIMEType:   mimeType,
			SizeBytes:  len(b),
			ContentB64: base64.StdEncoding.EncodeToString(b),
		})
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		switch h := part.Header.(type) {
		case *mail.InlineHeader:
			b, _ := io.ReadAll(part.Body)
			ct, params, _ := h.ContentType()
			switch {
			case strings.HasPrefix(ct, "text/html"):
				result.HTML = string(b)
			case strings.HasPrefix(ct, "text/"):
				// First text part wins: Gmail lists text/plain before text/html,
				// and a quoted reply's second text part must not replace it.
				if result.Text == "" {
					result.Text = string(b)
				}
			default:
				// Photos mailed from phones are Content-Disposition: inline
				// (multipart/related) and reach here; they are attachments, not
				// the body.
				keep(inlineFilename(h, params), ct, b)
			}
		case *mail.AttachmentHeader:
			b, _ := io.ReadAll(part.Body)
			filename, _ := h.Filename()
			mimeType, _, _ := h.ContentType()
			keep(filename, mimeType, b)
		}
	}
	result.Text = strings.TrimSpace(result.Text)
	result.HTML = strings.TrimSpace(result.HTML)
	return result
}

// inlineFilename returns the file name of an inline part: the
// Content-Disposition filename when present, else the Content-Type name.
func inlineFilename(h *mail.InlineHeader, ctParams map[string]string) string {
	if _, params, err := mime.ParseMediaType(h.Get("Content-Disposition")); err == nil {
		if name := strings.TrimSpace(params["filename"]); name != "" {
			return name
		}
	}
	return strings.TrimSpace(ctParams["name"])
}

// headerValues collects every value of any of the given header keys, in
// header order, lowercased, trimmed and de-duplicated.
func headerValues(h mail.Header, keys ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, key := range keys {
		fields := h.FieldsByKey(key)
		for fields.Next() {
			v := strings.ToLower(strings.TrimSpace(fields.Value()))
			if v == "" || seen[v] {
				continue
			}
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
