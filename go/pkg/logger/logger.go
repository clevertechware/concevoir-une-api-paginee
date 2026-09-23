// Package logger provides the logging abstraction used across the application.
//
// The interface exists so that tests can inject a silent implementation
// (NoOpLogger) without capturing stdout.
package logger

import "context"

// LoggingConfig holds the configuration for the logger.
type LoggingConfig struct {
	Level  string `koanf:"level"`
	Format string `koanf:"format"`
}

// Level is a severity threshold. It mirrors the four levels the configuration
// accepts, without exposing slog to the rest of the application.
type Level int

// The levels, ordered from the most verbose to the most severe.
const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Logger is the interface that wraps basic logging methods.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)

	// With returns a Logger that includes args in every subsequent record.
	With(args ...any) Logger

	// Enabled reports whether a record at this level would be emitted, so that
	// a caller can skip work whose only purpose is a record that gets dropped.
	Enabled(level Level) bool

	DebugContext(ctx context.Context, msg string, args ...any)
	InfoContext(ctx context.Context, msg string, args ...any)
	WarnContext(ctx context.Context, msg string, args ...any)
	ErrorContext(ctx context.Context, msg string, args ...any)
}
