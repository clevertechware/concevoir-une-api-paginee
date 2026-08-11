package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/domain"
)

func (s *RepositorySuite) TestFirstPage_ReturnsTheNewestRowsFirst() {
	t := s.T()

	rows, err := s.repository.FirstPage(t.Context(), domain.ListQuery{Sort: domain.SortCreatedAtDesc}, 3)

	require.NoError(t, err)
	assert.Equal(t, []int64{seededRows, seededRows - 1, seededRows - 2}, idsOf(rows))
	assert.True(t, createdAtOf(seededRows).Equal(rows[0].CreatedAt))
}

func (s *RepositorySuite) TestFirstPage_ReturnsTheOldestRowsFirstWhenAscending() {
	t := s.T()

	rows, err := s.repository.FirstPage(t.Context(), domain.ListQuery{Sort: domain.SortCreatedAtAsc}, 3)

	require.NoError(t, err)
	assert.Equal(t, []int64{1, 2, 3}, idsOf(rows))
}

func (s *RepositorySuite) TestFirstPage_RestrictsToTheAccountWhenFiltered() {
	t := s.T()

	rows, err := s.repository.FirstPage(
		t.Context(), domain.ListQuery{AccountID: 42, Sort: domain.SortCreatedAtDesc}, 10)

	require.NoError(t, err)
	require.Len(t, rows, 10)
	for _, row := range rows {
		assert.Equal(t, int64(42), row.AccountID)
	}
}

// TestNextPage_ResumesExactlyAfterTheBound is the tuple comparison at work.
// The bound is exclusive on the pair, not on either column: the row that
// produced it is never returned twice, and the one right after it is never
// skipped.
func (s *RepositorySuite) TestNextPage_ResumesExactlyAfterTheBound() {
	t := s.T()
	query := domain.ListQuery{Sort: domain.SortCreatedAtDesc}

	first, err := s.repository.FirstPage(t.Context(), query, 3)
	require.NoError(t, err)

	last := first[len(first)-1]
	second, err := s.repository.NextPage(
		t.Context(), query, domain.Bound{CreatedAt: last.CreatedAt, ID: last.ID}, 3)

	require.NoError(t, err)
	assert.Equal(t, []int64{last.ID - 1, last.ID - 2, last.ID - 3}, idsOf(second))
	assert.NotContains(t, idsOf(second), last.ID, "the bound is exclusive")
}

func (s *RepositorySuite) TestNextPage_ResumesUpwardsWhenAscending() {
	t := s.T()
	query := domain.ListQuery{Sort: domain.SortCreatedAtAsc}

	first, err := s.repository.FirstPage(t.Context(), query, 3)
	require.NoError(t, err)

	last := first[len(first)-1]
	second, err := s.repository.NextPage(
		t.Context(), query, domain.Bound{CreatedAt: last.CreatedAt, ID: last.ID}, 3)

	require.NoError(t, err)
	assert.Equal(t, []int64{last.ID + 1, last.ID + 2, last.ID + 3}, idsOf(second))
}

func (s *RepositorySuite) TestNextPage_StaysInsideTheAccountPartition() {
	t := s.T()
	query := domain.ListQuery{AccountID: 42, Sort: domain.SortCreatedAtDesc}

	first, err := s.repository.FirstPage(t.Context(), query, 5)
	require.NoError(t, err)

	last := first[len(first)-1]
	second, err := s.repository.NextPage(
		t.Context(), query, domain.Bound{CreatedAt: last.CreatedAt, ID: last.ID}, 5)

	require.NoError(t, err)
	require.Len(t, second, 5)
	for _, row := range second {
		assert.Equal(t, int64(42), row.AccountID)
		assert.Less(t, row.ID, last.ID)
	}
}

func (s *RepositorySuite) TestNextPage_ReturnsNothingPastTheLastRow() {
	t := s.T()

	rows, err := s.repository.NextPage(
		t.Context(),
		domain.ListQuery{Sort: domain.SortCreatedAtDesc},
		domain.Bound{CreatedAt: createdAtOf(1), ID: 1},
		20,
	)

	require.NoError(t, err)
	assert.Empty(t, rows)
}

func (s *RepositorySuite) TestOffsetPage_ReturnsTheSameRowsAsTheKeysetOnAStillTable() {
	t := s.T()
	query := domain.ListQuery{Sort: domain.SortCreatedAtDesc}

	byOffset, err := s.repository.OffsetPage(t.Context(), 0, 20, 10)
	require.NoError(t, err)

	first, err := s.repository.FirstPage(t.Context(), query, 20)
	require.NoError(t, err)
	last := first[len(first)-1]
	byKeyset, err := s.repository.NextPage(
		t.Context(), query, domain.Bound{CreatedAt: last.CreatedAt, ID: last.ID}, 10)
	require.NoError(t, err)

	// Nothing is inserted between the two calls, so the two paginations agree.
	// TestKeysetWalk_DoesNotDrift is where they stop agreeing.
	assert.Equal(t, idsOf(byKeyset), idsOf(byOffset))
}

// TestCountEstimate_ApproximatesTheTableWithoutCountingIt is the honest answer
// to a client asking for a total: reltuples is a single catalogue lookup, where
// COUNT(*) reads the whole table.
func (s *RepositorySuite) TestCountEstimate_ApproximatesTheTableWithoutCountingIt() {
	t := s.T()

	estimate, err := s.repository.CountEstimate(t.Context())

	require.NoError(t, err)
	assert.InEpsilon(t, float64(seededRows), float64(estimate), 0.05,
		"the planner estimate should land within 5%% of the real count")
}

func (s *RepositorySuite) TestPing_ReportsAReachablePool() {
	assert.NoError(s.T(), s.repository.Ping(s.T().Context()))
}

func (s *RepositorySuite) TestQueries_AreOneStatementPerCaseAndNeverShareABoundedForm() {
	t := s.T()

	// The article's trap is a single statement that neutralises its own bound
	// with `IS NULL`. Neither production statement may contain one: that is what
	// TestExplain_TheSingleQueryTrap measures the cost of.
	for name, query := range map[string]string{
		"descending first page":            descendingQueries.firstPage,
		"descending next page":             descendingQueries.nextPage,
		"descending first page by account": descendingQueries.firstPageByAccount,
		"descending next page by account":  descendingQueries.nextPageByAccount,
		"ascending first page":             ascendingQueries.firstPage,
		"ascending next page":              ascendingQueries.nextPage,
		"ascending first page by account":  ascendingQueries.firstPageByAccount,
		"ascending next page by account":   ascendingQueries.nextPageByAccount,
	} {
		t.Run(name, func(t *testing.T) {
			assert.NotContains(t, query, "IS NULL")
		})
	}
}

func TestQueriesFor_PicksTheDirectionFromTheSort(t *testing.T) {
	t.Parallel()

	assert.Equal(t, descendingQueries, queriesFor(domain.SortCreatedAtDesc))
	assert.Equal(t, ascendingQueries, queriesFor(domain.SortCreatedAtAsc))
	assert.Equal(t, descendingQueries, queriesFor(""), "an unset sort defaults to descending")
}
