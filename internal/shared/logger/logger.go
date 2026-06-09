package logger

import (
	"errors"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	Level      string
	Format     string
	OutputPath string
}

var (
	mu     sync.RWMutex
	global *zap.Logger
)

func Init(cfg Config) error {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return err
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	zapCfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(level),
		Development:      cfg.Format == "console",
		Encoding:         encodingFor(cfg.Format),
		EncoderConfig:    encoderCfg,
		OutputPaths:      outputs(cfg.OutputPath),
		ErrorOutputPaths: []string{"stderr"},
	}

	l, err := zapCfg.Build(zap.AddCaller(), zap.AddCallerSkip(0))
	if err != nil {
		return err
	}

	mu.Lock()
	global = l
	mu.Unlock()
	return nil
}

func Get() *zap.Logger {
	mu.RLock()
	defer mu.RUnlock()
	if global == nil {
		return zap.NewNop()
	}
	return global
}

func parseLevel(s string) (zapcore.Level, error) {
	if s == "" {
		return zapcore.InfoLevel, nil
	}
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(s)); err != nil {
		return zapcore.InfoLevel, errors.New("invalid log level: " + s)
	}
	return lvl, nil
}

func encodingFor(format string) string {
	if format == "console" {
		return "console"
	}
	return "json"
}

func outputs(path string) []string {
	if path == "" {
		return []string{"stdout"}
	}
	return []string{path}
}
