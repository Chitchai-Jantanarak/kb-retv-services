package logger

import (
	"os"

	"github.com/rs/zerolog"
)

func New() zerolog.Logger {
	logger := zerolog.New(os.Stdout).With().
		Timestamp().
		Str("service", "Driver").
		Logger()
	return logger
}
