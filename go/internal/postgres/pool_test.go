package postgres

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/testutil"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/logger"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/logger/mocks"
)

func TestNewPool_ReportsEveryStatementWhenTheApplicationRunsAtDebug(t *testing.T) {
	pg := testutil.Shared(t)

	var (
		mu      sync.Mutex
		queries []string
	)

	log := mocks.NewLogger(t)
	log.EXPECT().Enabled(logger.LevelDebug).Return(true).Once()
	log.EXPECT().
		DebugContext(mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, msg string, args ...any) {
			if msg != "Query" {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			queries = append(queries, sqlOf(args))
		}).
		Maybe()

	pool, err := NewPool(t.Context(), pg.Config, log)
	require.NoError(t, err)
	defer pool.Close()

	_, err = pool.Exec(t.Context(), "SELECT count(*) FROM transactions WHERE account_id = $1", 42)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, queries, "SELECT count(*) FROM transactions WHERE account_id = $1")
}

// TestNewPool_StaysSilentAtAnyOtherLevel is the reason the decision is taken
// once at startup: pgx allocates on every query as soon as a tracer exists.
func TestNewPool_StaysSilentAtAnyOtherLevel(t *testing.T) {
	pg := testutil.Shared(t)

	pool, err := NewPool(t.Context(), pg.Config, logger.New(logger.LoggingConfig{Level: "info"}))
	require.NoError(t, err)
	defer pool.Close()

	assert.Nil(t, pool.Config().ConnConfig.Tracer)
}

func sqlOf(args []any) string {
	for i := 0; i+1 < len(args); i += 2 {
		if args[i] == "sql" {
			if sql, ok := args[i+1].(string); ok {
				return sql
			}
		}
	}
	return ""
}
