package logger

import (
	"context"
	"log/slog"
	"os"
)

// SlogLogger implements Logger on top of the standard library slog package.
type SlogLogger struct {
	logger *slog.Logger
}

// NewSlogLogger creates a new SlogLogger with the given configuration.
func NewSlogLogger(config LoggingConfig) *SlogLogger {
	level := parseLevel(config.Level)

	var handler slog.Handler
	switch config.Format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	default:
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}

	return &SlogLogger{logger: slog.New(handler)}
}

// parseLevel converts a configuration string to a slog.Level, defaulting to info.
func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (l *SlogLogger) Debug(msg string, args ...any) { l.logger.Debug(msg, args...) }
func (l *SlogLogger) Info(msg string, args ...any)  { l.logger.Info(msg, args...) }
func (l *SlogLogger) Warn(msg string, args ...any)  { l.logger.Warn(msg, args...) }
func (l *SlogLogger) Error(msg string, args ...any) { l.logger.Error(msg, args...) }

// With delegates to slog, which already copies the attribute set. Keeping our own
// []any of fields would risk aliasing the backing array across derived loggers.
func (l *SlogLogger) With(args ...any) Logger {
	return &SlogLogger{logger: l.logger.With(args...)}
}

// Enabled asks the handler, so it follows the configured level. The context
// plays no part: both handlers this package installs ignore it.
func (l *SlogLogger) Enabled(level Level) bool {
	return l.logger.Enabled(context.Background(), slogLevel(level))
}

func slogLevel(level Level) slog.Level {
	switch level {
	case LevelDebug:
		return slog.LevelDebug
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (l *SlogLogger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.logger.DebugContext(ctx, msg, args...)
}

func (l *SlogLogger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.logger.InfoContext(ctx, msg, args...)
}

func (l *SlogLogger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.logger.WarnContext(ctx, msg, args...)
}

func (l *SlogLogger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.logger.ErrorContext(ctx, msg, args...)
}
