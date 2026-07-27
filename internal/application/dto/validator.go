package dto

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"

	apperr "github.com/my/app/internal/shared/errors"
)

var (
	validateOnce sync.Once
	validate     *validator.Validate
)

func validateStruct(value any) error {
	validateOnce.Do(func() {
		validate = validator.New(validator.WithRequiredStructEnabled())
	})

	if err := validate.Struct(value); err != nil {
		return apperr.Wrap(apperr.CodeInvalidInput, "invalid request", err)
	}
	return nil
}

func requireBodyOrAttachment(body string, attachments []AttachmentRef) error {
	if strings.TrimSpace(body) == "" && len(attachments) == 0 {
		return apperr.New(apperr.CodeInvalidInput, "body or attachment is required")
	}
	return nil
}

func normalizeText(value string) string {
	return strings.TrimSpace(value)
}

func validateAttachmentRefs(attachments []AttachmentRef) error {
	for i := range attachments {
		attachments[i].Normalize()
		if err := attachments[i].Validate(); err != nil {
			return err
		}
	}
	return nil
}

type AttachmentRef struct {
	ID         string `json:"id,omitempty"`
	URL        string `json:"url,omitempty"`
	StorageKey string `json:"storage_key,omitempty"`
	MIMEType   string `json:"mime_type,omitempty"`
	SizeBytes  int64  `json:"size_bytes,omitempty" validate:"omitempty,gte=0"`
}

func (r *AttachmentRef) Normalize() {
	r.ID = normalizeText(r.ID)
	r.URL = normalizeText(r.URL)
	r.StorageKey = normalizeText(r.StorageKey)
	r.MIMEType = normalizeText(r.MIMEType)
}

func (r AttachmentRef) Validate() error {
	if strings.HasPrefix(strings.ToLower(r.URL), "data:") {
		return apperr.New(apperr.CodeInvalidInput, "attachment url must reference stored media")
	}
	if r.URL == "" && r.StorageKey == "" && r.ID == "" {
		return apperr.New(apperr.CodeInvalidInput, "attachment reference is required")
	}
	if r.URL != "" {
		if strings.HasPrefix(r.URL, "//") {
			return apperr.New(apperr.CodeInvalidInput, "attachment url is invalid")
		}
		if strings.HasPrefix(r.URL, "/") {
			if _, err := url.Parse(r.URL); err != nil {
				return apperr.New(apperr.CodeInvalidInput, "attachment url is invalid")
			}
		} else {
			parsed, err := url.ParseRequestURI(r.URL)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return apperr.New(apperr.CodeInvalidInput, "attachment url is invalid")
			}
		}
	}
	if err := validateStruct(r); err != nil {
		return fmt.Errorf("validate attachment: %w", err)
	}
	return nil
}
