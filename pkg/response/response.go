package response

import (
	"time"

	"github.com/rs/zerolog/log"
)

type Error struct {
	CodeResponse int       `json:"code_response,omitempty"`
	Message      string    `json:"message"`
	CodeMsg      *string   `json:"code,omitempty"`
	Data         any       `json:"data,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	Logger       *Logger   `json:"logger,omitempty"`
}

type Logger struct {
	Title string `json:"title"`
	Msg   string `json:"msg"`
	Type  status `json:"type"`
}

type status string

const (
	StatusPanic status = "panic"
	StatusFatal status = "fatal"
	StatusError status = "error"
	StatusInfo  status = "info"
	StatusWarn  status = "warn"
	StatusDebug status = "debug"
	StatusTrace status = "trace"
)

func logByStatus(l *Logger) {
	if l == nil {
		return
	}

	logger := log.With().Str("title", l.Title).Logger()

	switch l.Type {
	case StatusPanic:
		logger.Panic().Msg(l.Msg)
	case StatusFatal:
		logger.Fatal().Msg(l.Msg)
	case StatusError:
		logger.Error().Msg(l.Msg)
	case StatusInfo:
		logger.Info().Msg(l.Msg)
	case StatusWarn:
		logger.Warn().Msg(l.Msg)
	case StatusDebug:
		logger.Debug().Msg(l.Msg)
	case StatusTrace:
		logger.Trace().Msg(l.Msg)
	default:
		logger.Info().Msg(l.Msg)
	}
}

func NewErrorLogger(err *Logger) {
	logByStatus(err)
}

func NewError(err *Error) *Error {
	if err.Logger != nil {
		logByStatus(err.Logger)
	}

	now := time.Now()
	res := &Error{
		CodeResponse: err.CodeResponse,
		Message:      err.Message,
		CodeMsg:      err.CodeMsg,
		Timestamp:    now,
	}

	if err.Data != nil {
		res.Data = err.Data
	}
	return res
}

func NewSuccess(resp *Success) *Success {
	now := time.Now()
	res := &Success{
		CodeResponse: resp.CodeResponse,
		Message:      resp.Message,
		CodeMsg:      resp.CodeMsg,
		Timestamp:    now,
	}

	if resp.Data != nil {
		res.Data = resp.Data
	}
	return res
}

type Success struct {
	CodeResponse int       `json:"code_response,omitempty"`
	Message      string    `json:"message"`
	CodeMsg      *string   `json:"code,omitempty"`
	Data         any       `json:"data,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}
