package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/application/workflows/omnichannel"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/transport/http/response"
)

type inboundWorkflow interface {
	Run(ctx context.Context, n omnichannel.Normalized, raw []byte) (omnichannel.Result, error)
}

const (
	defaultMaxBody = 1 << 20
	emailMaxBody   = 12 << 20

	defaultSignatureHeader = "X-AI-Signature"
)

type InboundHandler struct {
	workflow    inboundWorkflow
	normalizers *omnichannel.NormalizerRegistry
	secret      []byte
	maxBody     int64
	verifiers   map[string]SignatureVerifier
	sigHeaders  map[string]string
}

type SignatureVerifier func(req *http.Request, raw []byte) error

type InboundOption func(*InboundHandler)

func NewInboundHandler(workflow inboundWorkflow, normalizers *omnichannel.NormalizerRegistry, opts ...InboundOption) *InboundHandler {
	h := &InboundHandler{workflow: workflow, normalizers: normalizers, maxBody: defaultMaxBody}
	for _, opt := range opts {
		if opt != nil {
			opt(h)
		}
	}
	if h.maxBody <= 0 {
		h.maxBody = defaultMaxBody
	}
	return h
}

func WithInboundWebhookSecret(secret string) InboundOption {
	return func(h *InboundHandler) {
		h.secret = []byte(strings.TrimSpace(secret))
	}
}

func WithInboundMaxBodyBytes(n int64) InboundOption {
	return func(h *InboundHandler) {
		h.maxBody = n
	}
}

func WithChannelVerifier(channel string, v SignatureVerifier) InboundOption {
	return func(h *InboundHandler) {
		if h.verifiers == nil {
			h.verifiers = make(map[string]SignatureVerifier)
		}
		h.verifiers[strings.ToLower(strings.TrimSpace(channel))] = v
	}
}

func WithChannelSignatureHeader(channel, header string) InboundOption {
	return func(h *InboundHandler) {
		if h.sigHeaders == nil {
			h.sigHeaders = make(map[string]string)
		}
		h.sigHeaders[strings.ToLower(strings.TrimSpace(channel))] = strings.TrimSpace(header)
	}
}

func (h *InboundHandler) Receive(c *echo.Context) error {
	channel := c.Param("channel")
	if channel == "" {
		return response.WriteError(c, apperr.New(apperr.CodeInvalidInput, "channel path param is required"))
	}

	normalizer, err := h.normalizers.Get(channel)
	if err != nil {
		return response.WriteError(c, apperr.Wrap(apperr.CodeInvalidInput, "unknown channel", err))
	}

	if header, required := h.expectedSignatureHeader(channel); required {
		if strings.TrimSpace(c.Request().Header.Get(header)) == "" {
			return response.WriteError(c, apperr.New(apperr.CodeUnauthorized, "invalid signature"))
		}
	}

	raw, err := h.readBody(c.Request().Body, h.maxBodyFor(channel))
	if err != nil {
		if errors.Is(err, errInboundBodyTooLarge) {
			return response.WriteError(c, apperr.New(apperr.CodePayloadTooLarge, "request body too large"))
		}
		return response.WriteError(c, apperr.Wrap(apperr.CodeInvalidInput, "read body", err))
	}

	if verifier, ok := h.verifiers[strings.ToLower(channel)]; ok {
		if err := verifier(c.Request(), raw); err != nil {
			return response.WriteError(c, err)
		}
	} else if err := h.verifySignature(c.Request(), raw); err != nil {
		return response.WriteError(c, err)
	}

	normalized, err := normalizer.Normalize(raw)
	if err != nil {
		return response.WriteError(c, apperr.Wrap(apperr.CodeInvalidInput, "normalize payload", err))
	}

	result, err := h.workflow.Run(c.Request().Context(), normalized, raw)
	if err != nil {
		if errors.Is(err, omnichannel.ErrAccountNotFound) {
			return response.WriteError(c, apperr.Wrap(apperr.CodeNotFound, "unknown receiver", err))
		}
		return response.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, response.OK(map[string]any{
		"company_id":      result.CompanyID,
		"conversation_id": result.ConversationID,
		"message_id":      result.MessageID,
		"ticket_enqueued": result.TicketEnqueued,
	}))
}

var errInboundBodyTooLarge = errors.New("inbound body too large")

func (h *InboundHandler) maxBodyFor(channel string) int64 {
	if strings.EqualFold(strings.TrimSpace(channel), omnichannel.ChannelEmail) {
		return emailMaxBody
	}
	return h.maxBody
}

func (h *InboundHandler) expectedSignatureHeader(channel string) (header string, required bool) {
	key := strings.ToLower(strings.TrimSpace(channel))
	if header, ok := h.sigHeaders[key]; ok && header != "" {
		return header, true
	}
	if _, ok := h.verifiers[key]; ok {
		return "", false
	}
	if len(h.secret) == 0 {
		return "", false
	}
	return defaultSignatureHeader, true
}

func (h *InboundHandler) readBody(body io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errInboundBodyTooLarge
	}
	return raw, nil
}

func (h *InboundHandler) verifySignature(req *http.Request, raw []byte) error {
	if len(h.secret) == 0 {
		return nil
	}
	sig := strings.TrimSpace(req.Header.Get(defaultSignatureHeader))
	sig = strings.TrimPrefix(sig, "sha256=")
	if sig == "" {
		return apperr.New(apperr.CodeUnauthorized, "invalid signature")
	}
	given, err := hex.DecodeString(sig)
	if err != nil {
		return apperr.New(apperr.CodeUnauthorized, "invalid signature")
	}
	mac := hmac.New(sha256.New, h.secret)
	mac.Write(raw)
	if !hmac.Equal(given, mac.Sum(nil)) {
		return apperr.New(apperr.CodeUnauthorized, "invalid signature")
	}
	return nil
}
