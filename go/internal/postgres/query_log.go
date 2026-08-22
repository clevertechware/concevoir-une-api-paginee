package postgres

import (
	"context"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5/tracelog"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/logger"
)

// newQueryTracer builds the pgx tracer that reports every statement, its
// arguments and its round-trip duration.
//
// It exists for the reader of the article: the claim that a keyset page costs
// two distinct queries, and that neither of them grows an OFFSET, is easier to
// believe from the log of a running server than from the source.
func newQueryTracer(log logger.Logger) *tracelog.TraceLog {
	return &tracelog.TraceLog{
		Logger:   queryLogger{log: log},
		LogLevel: tracelog.LogLevelDebug,
		// pgx names the round-trip duration "time" by default, which is the key
		// slog already uses for the record timestamp: one record, two values,
		// and a duplicate key once the format is JSON.
		Config: &tracelog.TraceLogConfig{TimeKey: "duration"},
	}
}

type queryLogger struct {
	log logger.Logger
}

// Log flattens pgx's trace record into our structured logger. Keys are sorted
// because Go randomises map iteration, and a debug log read line by line should
// not shuffle its columns between two statements.
func (q queryLogger) Log(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]any) {
	args := make([]any, 0, 2*len(data))
	for _, key := range slices.Sorted(maps.Keys(data)) {
		args = append(args, key, data[key])
	}

	switch level {
	case tracelog.LogLevelError:
		q.log.ErrorContext(ctx, msg, args...)
	case tracelog.LogLevelWarn:
		q.log.WarnContext(ctx, msg, args...)
	default:
		// pgx reports a successful query at its own info level. Here every
		// statement is debug material: at info the tracer is not even installed.
		q.log.DebugContext(ctx, msg, args...)
	}
}
