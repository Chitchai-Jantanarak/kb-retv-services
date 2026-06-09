package errors

import (
	"errors"
	"fmt"
)

type Code string

const (
	CodeUnknown         Code = "unknown"
	CodeInvalidInput    Code = "invalid_input"
	CodeUnauthorized    Code = "unauthorized"
	CodeForbidden       Code = "forbidden"
	CodeNotFound        Code = "not_found"
	CodeConflict        Code = "conflict"
	CodeUnavailable     Code = "unavailable"
	CodeRateLimited     Code = "rate_limited"
	CodePayloadTooLarge Code = "payload_too_large"
	CodeInternal        Code = "internal"
	CodeMissingTenant   Code = "missing_tenant"
	CodeUpstreamFailed  Code = "upstream_failed"
)

type AppError struct {
	Code    Code
	Message string
	Cause   error
}

func (e *AppError) Error() string {
	if e.Cause == nil {
		return string(e.Code) + ": " + e.Message
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

func New(code Code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

func Wrap(code Code, message string, cause error) *AppError {
	return &AppError{Code: code, Message: message, Cause: cause}
}

func As(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}
