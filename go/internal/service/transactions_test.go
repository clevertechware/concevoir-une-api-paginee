package service

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/config"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/domain"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/logger"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/cursor"
)

var (
	testKey   = "concevoir-une-api-paginee-test-key"
	testEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	errRepo   = errors.New("database is on fire")
)

// stubRepository records what the service asked for and replays what it was
// told to return. Hand-written rather than generated: the port has five
// methods, and the assertions are about the arguments, not the call graph.
type stubRepository struct {
	rows []domain.Transaction
	err  error

	firstPageCalls int
	nextPageCalls  int
	gotQuery       domain.ListQuery
	gotBound       domain.Bound
	gotLimit       int
	gotOffset      int
	gotAfterID     int64
}

func (r *stubRepository) FirstPage(
	_ context.Context, q domain.ListQuery, limit int,
) ([]domain.Transaction, error) {
	r.firstPageCalls++
	r.gotQuery, r.gotLimit = q, limit
	return r.rows, r.err
}

func (r *stubRepository) NextPage(
	_ context.Context, q domain.ListQuery, after domain.Bound, limit int,
) ([]domain.Transaction, error) {
	r.nextPageCalls++
	r.gotQuery, r.gotBound, r.gotLimit = q, after, limit
	return r.rows, r.err
}

func (r *stubRepository) OffsetPage(
	_ context.Context, _ int64, offset, limit int,
) ([]domain.Transaction, error) {
	r.gotOffset, r.gotLimit = offset, limit
	return r.rows, r.err
}

func (r *stubRepository) Export(_ context.Context, afterID int64, limit int) ([]domain.Transaction, error) {
	r.gotAfterID, r.gotLimit = afterID, limit
	return r.rows, r.err
}

func (r *stubRepository) CountEstimate(context.Context) (int64, error) {
	return 10_000_000, r.err
}

func newService(t *testing.T, repository *stubRepository) *Transactions {
	t.Helper()

	s := NewTransactions(repository, config.Cursor{Key: testKey, TTL: 72 * time.Hour}, logger.NewNoOpLogger())
	s.now = func() time.Time { return testEpoch }
	return s
}

func transactions(n int) []domain.Transaction {
	rows := make([]domain.Transaction, 0, n)
	for i := 1; i <= n; i++ {
		rows = append(rows, domain.Transaction{
			ID:          int64(i),
			AccountID:   42,
			AmountCents: int64(i) * 100,
			Label:       "VIREMENT",
			CreatedAt:   time.Date(2021, 1, 8, 1, 47, 0, 0, time.UTC).Add(time.Duration(i) * time.Second),
		})
	}
	return rows
}

var descendingQuery = domain.ListQuery{AccountID: 42, Sort: domain.SortCreatedAtDesc}

// TestList_AsksForOneRowMoreThanThePageAndDropsIt is the has_more mechanism of
// the article: the extra row is the whole cost of knowing whether a next page
// exists, and it never leaves the server.
func TestList_AsksForOneRowMoreThanThePageAndDropsIt(t *testing.T) {
	tests := []struct {
		name        string
		available   int
		limit       int
		wantRows    int
		wantHasMore bool
		wantNext    bool
	}{
		{name: "a full page with a row to spare", available: 21, limit: 20, wantRows: 20, wantHasMore: true, wantNext: true},
		{name: "exactly one page and nothing after", available: 20, limit: 20, wantRows: 20},
		{name: "a partial last page", available: 7, limit: 20, wantRows: 7},
		{name: "no rows at all", available: 0, limit: 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &stubRepository{rows: transactions(tt.available)}
			s := newService(t, repository)

			page, err := s.List(t.Context(), descendingQuery, tt.limit, "")

			require.NoError(t, err)
			assert.Equal(t, tt.limit+1, repository.gotLimit, "the repository must be asked for limit + 1")
			assert.Len(t, page.Transactions, tt.wantRows)
			assert.Equal(t, tt.wantHasMore, page.HasMore)
			assert.Equal(t, tt.wantNext, page.Next != "", "next is the only authority on the end of the walk")
		})
	}
}

// TestList_UsesTheFirstPageQueryWithoutACursor pins the article's central
// point: which of the two statements runs is decided here, once, on the
// presence of a cursor. There is no third path where a bound is passed as NULL.
func TestList_UsesTheFirstPageQueryWithoutACursor(t *testing.T) {
	repository := &stubRepository{rows: transactions(5)}
	s := newService(t, repository)

	_, err := s.List(t.Context(), descendingQuery, 20, "")

	require.NoError(t, err)
	assert.Equal(t, 1, repository.firstPageCalls)
	assert.Equal(t, 0, repository.nextPageCalls)
}

func TestList_UsesTheNextPageQueryWithACursor(t *testing.T) {
	repository := &stubRepository{rows: transactions(21)}
	s := newService(t, repository)

	first, err := s.List(t.Context(), descendingQuery, 20, "")
	require.NoError(t, err)
	require.NotEmpty(t, first.Next)

	second, err := s.List(t.Context(), descendingQuery, 20, first.Next)
	require.NoError(t, err)
	require.NotNil(t, second)

	assert.Equal(t, 1, repository.nextPageCalls)
	last := first.Transactions[len(first.Transactions)-1]
	assert.Equal(t, last.ID, repository.gotBound.ID, "the bound is the last row handed to the client")
	assert.True(t, last.CreatedAt.Equal(repository.gotBound.CreatedAt))
}

// TestList_RejectsACursorThatDoesNotBelongToTheRequest covers two claims at
// once: a token is unforgeable, and it is bound to the filters that produced it.
func TestList_RejectsACursorThatDoesNotBelongToTheRequest(t *testing.T) {
	repository := &stubRepository{rows: transactions(21)}
	s := newService(t, repository)

	issued, err := s.List(t.Context(), descendingQuery, 20, "")
	require.NoError(t, err)
	require.NotEmpty(t, issued.Next)

	tests := []struct {
		name    string
		query   domain.ListQuery
		token   string
		wantErr error
	}{
		{
			name:  "replays the token on the query that issued it",
			query: descendingQuery,
			token: issued.Next,
		},
		{
			name:    "replays the token on another account",
			query:   domain.ListQuery{AccountID: 43, Sort: domain.SortCreatedAtDesc},
			token:   issued.Next,
			wantErr: cursor.ErrFilterMismatch,
		},
		{
			name:    "replays the token with no account filter at all",
			query:   domain.ListQuery{Sort: domain.SortCreatedAtDesc},
			token:   issued.Next,
			wantErr: cursor.ErrFilterMismatch,
		},
		{
			name:    "replays the token backwards",
			query:   domain.ListQuery{AccountID: 42, Sort: domain.SortCreatedAtAsc},
			token:   issued.Next,
			wantErr: cursor.ErrFilterMismatch,
		},
		{
			name:    "replays the token with another status filter",
			query:   domain.ListQuery{AccountID: 42, Status: "SETTLED", Sort: domain.SortCreatedAtDesc},
			token:   issued.Next,
			wantErr: cursor.ErrFilterMismatch,
		},
		{
			name:    "submits a token with one byte changed",
			query:   descendingQuery,
			token:   tamper(t, issued.Next),
			wantErr: cursor.ErrInvalidCursor,
		},
		{
			name:    "submits something that is not a token",
			query:   descendingQuery,
			token:   "definitely-not-a-cursor!!",
			wantErr: cursor.ErrInvalidCursor,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.List(t.Context(), tt.query, 20, tt.token)

			if tt.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestList_ExpiresACursorPastItsTTL is what the 410 of the contract rests on.
// The same token is accepted or refused purely on how much time has passed, so
// the clock is the only thing that moves between the two cases.
func TestList_ExpiresACursorPastItsTTL(t *testing.T) {
	repository := &stubRepository{rows: transactions(21)}
	s := newService(t, repository)

	issued, err := s.List(t.Context(), descendingQuery, 20, "")
	require.NoError(t, err)
	require.NotEmpty(t, issued.Next)

	tests := []struct {
		name    string
		elapsed time.Duration
		wantErr error
	}{
		{name: "resumes an hour later", elapsed: time.Hour},
		{name: "resumes on the last hour of the TTL", elapsed: 71 * time.Hour},
		{name: "refuses to resume a day too late", elapsed: 96 * time.Hour, wantErr: cursor.ErrCursorExpired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s.now = func() time.Time { return testEpoch.Add(tt.elapsed) }

			_, err := s.List(t.Context(), descendingQuery, 20, issued.Next)

			if tt.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestList_PropagatesRepositoryFailures(t *testing.T) {
	s := newService(t, &stubRepository{err: errRepo})

	_, err := s.List(t.Context(), descendingQuery, 20, "")

	assert.ErrorIs(t, err, errRepo)
}

func TestListByOffset_TranslatesThePageNumberIntoARowsToSkipCount(t *testing.T) {
	tests := []struct {
		name       string
		page       int
		size       int
		wantOffset int
	}{
		{name: "the first page skips nothing", page: 1, size: 20, wantOffset: 0},
		{name: "the second page skips one page", page: 2, size: 20, wantOffset: 20},
		{name: "the article's worst case", page: 250_001, size: 20, wantOffset: 5_000_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &stubRepository{rows: transactions(tt.size + 1)}
			s := newService(t, repository)

			page, err := s.ListByOffset(t.Context(), 0, tt.page, tt.size)

			require.NoError(t, err)
			assert.Equal(t, tt.wantOffset, repository.gotOffset)
			assert.Equal(t, tt.size+1, repository.gotLimit)
			assert.Equal(t, tt.page, page.Page)
			assert.True(t, page.HasMore)
		})
	}
}

func TestExport_ResumesFromTheLastIdentifierItReturned(t *testing.T) {
	repository := &stubRepository{rows: transactions(11)}
	s := newService(t, repository)

	page, err := s.Export(t.Context(), 0, 10)

	require.NoError(t, err)
	assert.Equal(t, int64(0), repository.gotAfterID, "an absent after_id starts the walk")
	assert.Equal(t, 11, repository.gotLimit)
	assert.Len(t, page.Transactions, 10)
	assert.True(t, page.HasMore)
	assert.Equal(t, page.Transactions[9].ID, page.NextAfterID)

	repository.rows = transactions(3)
	page, err = s.Export(t.Context(), page.NextAfterID, 10)

	require.NoError(t, err)
	assert.Equal(t, int64(10), repository.gotAfterID)
	assert.False(t, page.HasMore)
	assert.Zero(t, page.NextAfterID, "the walk is over, there is nothing to resume from")
}

func TestCountEstimate_ReturnsThePlannerEstimate(t *testing.T) {
	s := newService(t, &stubRepository{})

	estimate, err := s.CountEstimate(t.Context())

	require.NoError(t, err)
	assert.Equal(t, int64(10_000_000), estimate)
}

// tamper flips one bit of the signed payload and re-encodes.
//
// Deliberately not "change the last character of the token": base64 does not
// use every bit of its final character, so such a change can decode to exactly
// the same bytes and leave the token valid. The forgery has to happen on the
// bytes the signature covers.
func tamper(t *testing.T, token string) string {
	t.Helper()

	raw, err := base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err)

	raw[len(raw)-1] ^= 0x01
	return base64.RawURLEncoding.EncodeToString(raw)
}
