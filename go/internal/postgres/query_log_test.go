package postgres

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/tracelog"
	"github.com/stretchr/testify/mock"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/logger/mocks"
)

// The expectation is the whole flattened record, so it pins the key order too:
// pgx hands over a map, and an unsorted one would shuffle the columns of a log
// read line by line.
func TestQueryLogger_SortsTheRecordAndKeepsTheSeverityOfAFailure(t *testing.T) {
	t.Parallel()

	const sql = "SELECT id FROM transactions WHERE account_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2"

	record := map[string]any{"sql": sql, "args": []any{int64(42), 21}, "duration": 3 * time.Millisecond}
	sorted := []any{"args", []any{int64(42), 21}, "duration", 3 * time.Millisecond, "sql", sql}

	tests := []struct {
		name   string
		level  tracelog.LogLevel
		expect func(log *mocks.Logger)
	}{
		{
			name:  "a statement that answered is debug material, whatever pgx calls it",
			level: tracelog.LogLevelInfo,
			expect: func(log *mocks.Logger) {
				log.EXPECT().DebugContext(mock.Anything, "Query", sorted).Once()
			},
		},
		{
			name:  "a statement that failed stays an error",
			level: tracelog.LogLevelError,
			expect: func(log *mocks.Logger) {
				log.EXPECT().ErrorContext(mock.Anything, "Query", sorted).Once()
			},
		},
		{
			name:  "a warning stays a warning",
			level: tracelog.LogLevelWarn,
			expect: func(log *mocks.Logger) {
				log.EXPECT().WarnContext(mock.Anything, "Query", sorted).Once()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			log := mocks.NewLogger(t)
			tt.expect(log)

			queryLogger{log: log}.Log(t.Context(), tt.level, "Query", record)
		})
	}
}
