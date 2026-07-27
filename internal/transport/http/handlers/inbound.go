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

type InboundHandler struct {
	workflow    inboundWorkflow
	normalizers *omnichannel.NormalizerRegistry
	secret      []byte
	maxBody     int64
	verifiers   map[string]SignatureVerifier
}

type SignatureVerifier func(req *http.Request, raw []byte) error

type InboundOption func(*InboundHandler)

func NewInboundHandler(workflow inboundWorkflow, normalizers *omnichannel.NormalizerRegistry, opts ...InboundOption) *InboundHandler {
	h := &InboundHandler{workflow: workflow, normalizers: normalizers, maxBody: 1 << 20}
	for _, opt := range opts {
		if opt != nil {
			opt(h)
		}
	}
	if h.maxBody <= 0 {
		h.maxBody = 1 << 20
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

func (h *InboundHandler) Receive(c *echo.Context) error {
	channel := c.Param("channel")
	if channel == "" {
		return response.WriteError(c, apperr.New(apperr.CodeInvalidInput, "channel path param is required"))
	}

	normalizer, err := h.normalizers.Get(channel)
	if err != nil {
		return response.WriteError(c, apperr.Wrap(apperr.CodeInvalidInput, "unknown channel", err))
	}

	raw, err := h.readBody(c.Request().Body)
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

func (h *InboundHandler) readBody(body io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, h.maxBody+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > h.maxBody {
		return nil, errInboundBodyTooLarge
	}
	return raw, nil
}

func (h *InboundHandler) verifySignature(req *http.Request, raw []byte) error {
	if len(h.secret) == 0 {
		return nil
	}
	sig := strings.TrimSpace(req.Header.Get("X-AI-Signature"))
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
