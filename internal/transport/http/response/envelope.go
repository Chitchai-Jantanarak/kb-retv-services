package response

import (
	"net/http"

	"github.com/labstack/echo/v5"

	apperr "github.com/my/app/internal/shared/errors"
)

type Envelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *Error      `json:"error,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func OK(data interface{}) Envelope {
	return Envelope{Success: true, Data: data}
}

func Fail(code apperr.Code, message string) Envelope {
	return Envelope{Success: false, Error: &Error{Code: string(code), Message: message}}
}

func WriteError(c *echo.Context, err error) error {
	if ae, ok := apperr.As(err); ok {
		return c.JSON(statusFor(ae.Code), Fail(ae.Code, ae.Message))
	}
	return c.JSON(http.StatusInternalServerError, Fail(apperr.CodeInternal, err.Error()))
}

func statusFor(code apperr.Code) int {
	switch code {
	case apperr.CodeInvalidInput:
		return http.StatusBadRequest
	case apperr.CodeUnauthorized, apperr.CodeMissingTenant:
		return http.StatusUnauthorized
	case apperr.CodeForbidden:
		return http.StatusForbidden
	case apperr.CodeNotFound:
		return http.StatusNotFound
	case apperr.CodeConflict:
		return http.StatusConflict
	case apperr.CodeRateLimited:
		return http.StatusTooManyRequests
	case apperr.CodePayloadTooLarge:
		return http.StatusRequestEntityTooLarge
	case apperr.CodeUnavailable:
		return http.StatusServiceUnavailable
	case apperr.CodeUpstreamFailed:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}
