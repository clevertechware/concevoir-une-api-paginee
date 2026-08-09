package postgres

import (
	"context"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests in this file assert on execution plans rather than on results.
// They are the ones that prove the article's claims: a result set can be
// perfectly correct and still have cost 65 890 blocks to produce.

// TestExplain_NextPageBoundIsAnIndexCond is the single most important plan
// assertion of the demo.
//
// When the bound appears under `Index Cond`, PostgreSQL uses it to position the
// scan before reading anything. Under `Filter`, it reads rows and discards them
// one by one — the behaviour of OFFSET, wearing keyset syntax.
func (s *RepositorySuite) TestExplain_NextPageBoundIsAnIndexCond() {
	t := s.T()

	tests := []struct {
		name  string
		query string
		args  []any
	}{
		{
			name:  "unfiltered walk",
			query: descendingQueries.nextPage,
			args:  []any{createdAtOf(50_000), int64(50_000), 20},
		},
		{
			name:  "walk filtered by account",
			query: descendingQueries.nextPageByAccount,
			args:  []any{int64(42), createdAtOf(50_042), int64(50_042), 20},
		},
		{
			name:  "ascending walk",
			query: ascendingQueries.nextPage,
			args:  []any{createdAtOf(50_000), int64(50_000), 20},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := s.explain(tt.query, tt.args...)

			assert.Regexp(t, `Index Cond:.*created_at`, plan,
				"the bound must position the scan, not filter it\n%s", plan)
			assert.NotContains(t, plan, "Rows Removed by Filter",
				"a keyset page must not read rows only to throw them away\n%s", plan)
		})
	}
}

// TestExplain_KeysetCostDoesNotGrowWithDepth is the claim that a keyset page
// costs the same on the first page and on the two hundred and fifty thousandth:
// a descent down the B-tree, then twenty rows read in sequence.
func (s *RepositorySuite) TestExplain_KeysetCostDoesNotGrowWithDepth() {
	t := s.T()

	shallow, shallowBlocks := s.measure(descendingQueries.nextPage,
		createdAtOf(seededRows-100), int64(seededRows-100), 20)
	deep, deepBlocks := s.measure(descendingQueries.nextPage,
		createdAtOf(100), int64(100), 20)

	t.Logf("blocks read: near the top=%d, %d rows deeper=%d", shallowBlocks, seededRows-200, deepBlocks)

	// Not equality: which pages sit in shared_buffers still varies a little.
	// The claim is that the cost is bounded, not that it is identical — an
	// OFFSET of the same depth reads three orders of magnitude more.
	assert.LessOrEqual(t, deepBlocks, shallowBlocks*3,
		"a cursor %d rows deeper must not cost proportionally more\nnear the top:\n%s\ndeep:\n%s",
		seededRows-200, shallow, deep)
	assert.Less(t, deepBlocks, 100,
		"a keyset page is a B-tree descent and twenty sequential rows, whatever its depth\n%s", deep)
}

// TestExplain_OffsetCostGrowsWithDepth is the same measurement on the
// counter-example, and the reason that endpoint exists at all.
func (s *RepositorySuite) TestExplain_OffsetCostGrowsWithDepth() {
	t := s.T()

	shallow, shallowBlocks := s.measure(offsetPage, 20, 0)
	deep, deepBlocks := s.measure(offsetPage, 20, seededRows-100)

	t.Logf("blocks read: OFFSET 0=%d, OFFSET %d=%d", shallowBlocks, seededRows-100, deepBlocks)

	assert.Greater(t, deepBlocks, shallowBlocks*5,
		"OFFSET pays for every row it skips, and that is the whole problem\nshallow:\n%s\ndeep:\n%s",
		shallow, deep)
}

// TestExplain_TheSingleQueryTrap demonstrates the trap rather than merely
// asserting that the production code avoids it.
//
// The tempting shortcut is one statement for every page:
//
//	WHERE account_id = 42 AND ($1 IS NULL OR (created_at, id) < ($1, $2))
//
// With a custom plan — parameter values known when the plan is built —
// PostgreSQL folds the IS NULL away and the bound still positions the scan.
// That is what makes the trap so hard to spot: it looks fine in psql.
//
// But a prepared statement reused a few times, which is the normal behaviour of
// most drivers including pgx, switches to a generic plan. There $1 is unknown
// at planning time, the predicate can no longer be an index bound, and it
// degrades into a Filter: the scan restarts from the top of the account's
// partition and discards rows one by one.
//
// `SET plan_cache_mode = force_generic_plan` reproduces that switch on demand,
// instead of executing the statement five times and hoping.
func (s *RepositorySuite) TestExplain_TheSingleQueryTrap() {
	t := s.T()

	// PREPARE, the plan cache mode and DEALLOCATE are all session state, so
	// everything below has to run on one connection.
	conn, err := s.pg.Pool.Acquire(t.Context())
	require.NoError(t, err)
	defer conn.Release()

	const trapQuery = `
		PREPARE paged (timestamptz, bigint) AS
		SELECT id, account_id, amount_cents, label, created_at
		FROM transactions
		WHERE account_id = 42
		  AND ($1 IS NULL OR (created_at, id) < ($1, $2))
		ORDER BY created_at DESC, id DESC
		LIMIT 20`

	_, err = conn.Exec(t.Context(), trapQuery)
	require.NoError(t, err)
	defer func() {
		// t.Context() is already cancelled by the time a deferred call runs on a
		// failing test, and a cancelled DEALLOCATE would leave the statement on
		// the connection for whoever gets it next.
		_, _ = conn.Exec(context.WithoutCancel(t.Context()), "DEALLOCATE paged")
	}()

	// A cursor halfway down account 42's partition: deep enough for the
	// difference between an index bound and a filter to show.
	const cursorID = 50_042
	execute := "EXECUTE paged ('" + createdAtOf(cursorID).Format("2006-01-02 15:04:05-07") + "', " +
		strconv.Itoa(cursorID) + ")"

	custom := explainPlan(t, conn, execute)
	assert.NotContains(t, custom, "Rows Removed by Filter",
		"the custom plan folds the IS NULL away, which is exactly why the trap goes unnoticed\n%s", custom)

	_, err = conn.Exec(t.Context(), "SET plan_cache_mode = force_generic_plan")
	require.NoError(t, err)

	generic := explainPlan(t, conn, execute)

	_, err = conn.Exec(t.Context(), "RESET plan_cache_mode")
	require.NoError(t, err)

	assert.Contains(t, generic, "Filter:",
		"the generic plan cannot use the bound as an index condition\n%s", generic)

	removed := rowsRemovedByFilter(t, generic)
	t.Logf("rows removed by filter under the generic plan: %d", removed)
	assert.Positive(t, removed,
		"this is OFFSET in keyset clothing: %d rows read only to be discarded\n%s", removed, generic)

	// And the statement the repository actually issues does not have the problem.
	twoStatementForm := s.explain(descendingQueries.nextPageByAccount,
		int64(42), createdAtOf(cursorID), int64(cursorID), 20)
	assert.NotContains(t, twoStatementForm, "Rows Removed by Filter",
		"splitting first page and next page keeps the bound an index condition\n%s", twoStatementForm)
}

var (
	// Buffer counters appear as `Buffers: shared hit=4 read=1`, sometimes
	// without the hit, sometimes with local or temp counters alongside.
	bufferCounters  = regexp.MustCompile(`(?:hit|read|dirtied|written)=(\d+)`)
	removedByFilter = regexp.MustCompile(`Rows Removed by Filter: (\d+)`)
)

// blocksRead sums every buffer counter in the plan. That total is the currency
// the article compares: 4 blocks for a keyset page at any depth, 65 890 for an
// OFFSET of five million.
func blocksRead(t *testing.T, plan string) int {
	t.Helper()

	total := 0
	for _, match := range bufferCounters.FindAllStringSubmatch(plan, -1) {
		value, err := strconv.Atoi(match[1])
		require.NoError(t, err)
		total += value
	}
	require.Positive(t, total, "no buffer counter found, was BUFFERS requested?\n%s", plan)

	return total
}

func rowsRemovedByFilter(t *testing.T, plan string) int {
	t.Helper()

	match := removedByFilter.FindStringSubmatch(plan)
	require.NotNil(t, match, "no Rows Removed by Filter in\n%s", plan)

	value, err := strconv.Atoi(match[1])
	require.NoError(t, err)

	return value
}
