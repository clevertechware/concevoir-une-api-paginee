package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/domain"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/logger"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/testutil"
)

const (
	// seededRows is enough for a tenant partition to be deep: the trap the
	// article documents only does visible damage once a cursor sits far from
	// the top of its partition.
	seededRows     = 100_000
	seededAccounts = 50
)

// epoch mirrors the seed: row i carries id i and created_at epoch + i × 20 s.
var epoch = time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)

func createdAtOf(id int64) time.Time {
	return epoch.Add(time.Duration(id) * 20 * time.Second)
}

// RepositorySuite runs the read-only assertions against a dataset seeded once.
// Nothing here writes, so no isolation is needed between tests.
type RepositorySuite struct {
	suite.Suite

	pg         *testutil.Postgres
	repository *TransactionRepository
}

func TestRepositorySuite(t *testing.T) {
	suite.Run(t, new(RepositorySuite))
}

func (s *RepositorySuite) SetupSuite() {
	s.pg = testutil.Shared(s.T())
	testutil.SeedTransactions(s.T(), s.pg, seededRows, seededAccounts)
	s.repository = NewTransactionRepository(s.pg.Pool, logger.NewNoOpLogger())
}

// querier is what both a pool and a single connection satisfy. The trap test
// needs a pinned connection, because PREPARE and plan_cache_mode are session state.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// explainPlan runs a statement under EXPLAIN and returns the plan as one string.
// ANALYZE means the numbers are measured rather than estimated, and BUFFERS is
// what turns "the cost does not grow with depth" into something assertable.
func explainPlan(t *testing.T, db querier, query string, args ...any) string {
	t.Helper()

	rows, err := db.Query(t.Context(),
		"EXPLAIN (ANALYZE, BUFFERS, COSTS OFF, TIMING OFF) "+query, args...)
	require.NoError(t, err)
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan.WriteString(line)
		plan.WriteString("\n")
	}
	require.NoError(t, rows.Err())

	return plan.String()
}

func (s *RepositorySuite) explain(query string, args ...any) string {
	s.T().Helper()
	return explainPlan(s.T(), s.pg.Pool, query, args...)
}

// measure returns a plan and its block count, after a warm-up run. Without the
// warm-up the first statement of a test pays for a cold shared_buffers and can
// report more blocks than the far deeper query it is being compared against.
func (s *RepositorySuite) measure(query string, args ...any) (plan string, blocks int) {
	s.T().Helper()

	s.explain(query, args...)
	plan = s.explain(query, args...)

	return plan, blocksRead(s.T(), plan)
}

func idsOf(transactions []domain.Transaction) []int64 {
	ids := make([]int64, 0, len(transactions))
	for _, t := range transactions {
		ids = append(ids, t.ID)
	}
	return ids
}
