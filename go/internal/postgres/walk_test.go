package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/domain"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/logger"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/testutil"
)

// The tests in this file walk a table that is being written to while the walk
// is in progress. They are the heart of the article: not "keyset is faster",
// but "OFFSET returns rows that were never asked for and hides rows that were,
// without ever raising an error".
//
// Each one seeds the dataset it needs, so the order in which Go runs them
// against the shared container does not matter.

const (
	walkRows     = 200
	walkPageSize = 20
	// insertsPerPage is how many rows land at the head of the ordering between
	// two pages: the "three new transactions arrive" of the article, per page.
	insertsPerPage = 3
)

// TestKeysetWalk_DoesNotDrift is the claim the whole contract is built on.
//
// A cursor names a position in the sort order, not a rank in a set. Rows
// inserted at the head of the ordering during the walk therefore do not move
// it: every row present when the walk started comes back exactly once, and none
// of the newcomers can push another row past the reader unseen.
func TestKeysetWalk_DoesNotDrift(t *testing.T) {
	pg := testutil.Shared(t)
	testutil.SeedTransactions(t, pg, walkRows, seededAccounts)
	repository := NewTransactionRepository(pg.Pool, logger.NewNoOpLogger())

	query := domain.ListQuery{Sort: domain.SortCreatedAtDesc}
	inserter := newHeadInserter(walkRows)

	seen := make([]int64, 0, walkRows)

	page, err := repository.FirstPage(t.Context(), query, walkPageSize)
	require.NoError(t, err)

	for len(page) > 0 {
		seen = append(seen, idsOf(page)...)
		inserter.insert(t, pg, insertsPerPage)

		last := page[len(page)-1]
		page, err = repository.NextPage(
			t.Context(), query, domain.Bound{CreatedAt: last.CreatedAt, ID: last.ID}, walkPageSize)
		require.NoError(t, err)
	}

	duplicates := duplicatesIn(seen)
	assert.Empty(t, duplicates, "a keyset walk must not return a row twice, got %v", duplicates)

	missing := missingFrom(seen, walkRows)
	assert.Empty(t, missing, "a keyset walk must not skip a row that existed when it started, missed %v", missing)
}

// TestOffsetWalk_Drifts is the same walk on the counter-example, and it fails
// in the way the article describes. Each insert at the head pushes the whole
// dataset down one rank, so the next `OFFSET n` lands on rows the reader has
// already been given.
//
// Nothing raises an error. That is the point: the client cannot detect this.
func TestOffsetWalk_Drifts(t *testing.T) {
	pg := testutil.Shared(t)
	testutil.SeedTransactions(t, pg, walkRows, seededAccounts)
	repository := NewTransactionRepository(pg.Pool, logger.NewNoOpLogger())

	inserter := newHeadInserter(walkRows)

	seen := make([]int64, 0, walkRows)
	for pageNumber := 0; pageNumber*walkPageSize < walkRows; pageNumber++ {
		page, err := repository.OffsetPage(t.Context(), 0, pageNumber*walkPageSize, walkPageSize)
		require.NoError(t, err)

		seen = append(seen, idsOf(page)...)
		inserter.insert(t, pg, insertsPerPage)
	}

	duplicates := duplicatesIn(seen)
	assert.NotEmpty(t, duplicates,
		"rows inserted at the head shift every rank down, so an OFFSET walk must repeat rows")

	// The rows returned twice are rows that some other row was pushed past, so
	// the same walk is also missing part of the original dataset.
	missing := missingFrom(seen, walkRows)
	assert.NotEmpty(t, missing,
		"and what it repeats, it repeats instead of something else it never returned")

	t.Logf("offset walk over %d rows: %d duplicates, %d rows never returned",
		walkRows, len(duplicates), len(missing))
}

// TestExportWalk_DoesNotDrift makes the same point on the strongest ordering
// available: a strictly increasing, never-updated primary key. Rows inserted
// during the walk land *after* the reader, so they are simply picked up later,
// and nothing that existed at the start can be missed.
func TestExportWalk_DoesNotDrift(t *testing.T) {
	pg := testutil.Shared(t)
	testutil.SeedTransactions(t, pg, walkRows, seededAccounts)
	repository := NewTransactionRepository(pg.Pool, logger.NewNoOpLogger())

	inserter := newHeadInserter(walkRows)

	seen := make([]int64, 0, walkRows)
	afterID := int64(0)

	for {
		page, err := repository.Export(t.Context(), afterID, walkPageSize)
		require.NoError(t, err)
		if len(page) == 0 {
			break
		}

		for _, row := range page {
			// Stop at the original dataset: everything above walkRows is a row
			// that did not exist when the walk started.
			if row.ID > walkRows {
				continue
			}
			seen = append(seen, row.ID)
		}
		afterID = page[len(page)-1].ID
		if afterID >= walkRows {
			break
		}

		inserter.insert(t, pg, insertsPerPage)
	}

	assert.Empty(t, duplicatesIn(seen), "an export walk must not return a row twice")
	assert.Empty(t, missingFrom(seen, walkRows), "an export walk must not skip a row")
}

// TestKeysetWalk_NeedsTheTieBreakerOnIdenticalTimestamps is why the sort key
// ends on a unique NOT NULL column. Every row here shares one created_at, so
// created_at alone gives no order at all: only the id in the ORDER BY and in
// the bound makes the walk total and repeat-free.
func TestKeysetWalk_NeedsTheTieBreakerOnIdenticalTimestamps(t *testing.T) {
	pg := testutil.Shared(t)
	const rows = 50
	testutil.SeedIdenticalTimestamps(t, pg, rows)
	repository := NewTransactionRepository(pg.Pool, logger.NewNoOpLogger())

	query := domain.ListQuery{Sort: domain.SortCreatedAtDesc}
	seen := make([]int64, 0, rows)

	page, err := repository.FirstPage(t.Context(), query, 10)
	require.NoError(t, err)

	for len(page) > 0 {
		seen = append(seen, idsOf(page)...)

		last := page[len(page)-1]
		page, err = repository.NextPage(
			t.Context(), query, domain.Bound{CreatedAt: last.CreatedAt, ID: last.ID}, 10)
		require.NoError(t, err)
	}

	assert.Len(t, seen, rows, "every row must be seen, even sharing a single created_at")
	assert.Empty(t, duplicatesIn(seen), "and none of them twice")
	assert.Equal(t, int64(rows), seen[0], "the id supplies the order created_at cannot")
	assert.Equal(t, int64(1), seen[len(seen)-1])
}

// headInserter adds rows at the head of the descending ordering, the way new
// transactions arrive while a client is paginating.
type headInserter struct {
	// createdAt keeps moving forward so every new row sorts above the previous one.
	createdAt time.Time
}

func newHeadInserter(seededRows int) *headInserter {
	return &headInserter{createdAt: createdAtOf(int64(seededRows))}
}

func (i *headInserter) insert(t *testing.T, pg *testutil.Postgres, count int) {
	t.Helper()

	for range count {
		i.createdAt = i.createdAt.Add(20 * time.Second)

		_, err := pg.Pool.Exec(context.WithoutCancel(t.Context()),
			`INSERT INTO transactions (account_id, amount_cents, label, created_at)
			 VALUES (42, 1000, 'ARRIVED MID WALK', $1)`, i.createdAt)
		require.NoError(t, err)
	}
}

func duplicatesIn(ids []int64) []int64 {
	counts := make(map[int64]int, len(ids))
	duplicates := make([]int64, 0)

	for _, id := range ids {
		counts[id]++
		if counts[id] == 2 {
			duplicates = append(duplicates, id)
		}
	}
	return duplicates
}

// missingFrom returns the identifiers of the original dataset that never came back.
func missingFrom(seen []int64, total int) []int64 {
	found := make(map[int64]struct{}, len(seen))
	for _, id := range seen {
		found[id] = struct{}{}
	}

	missing := make([]int64, 0)
	for id := int64(1); id <= int64(total); id++ {
		if _, ok := found[id]; !ok {
			missing = append(missing, id)
		}
	}
	return missing
}
