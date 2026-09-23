package handler

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/domain"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/handler/mocks"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/cursor"
)

// TestList_CapsTheLimitInsteadOfRejectingIt is the contract rule that a page
// size is clamped, never refused: a client asking for 10 000 rows gets 100 and
// a 200, not a 400 that only teaches it to retry. The rejected cases set no
// expectation at all, so reaching the service would fail the test.
func TestList_CapsTheLimitInsteadOfRejectingIt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		target     string
		wantStatus int
		wantLimit  int
		wantCode   string
	}{
		{name: "defaults to 20", target: "/v1/transactions", wantStatus: http.StatusOK, wantLimit: 20},
		{name: "honours a limit below the cap", target: "/v1/transactions?limit=50", wantStatus: http.StatusOK, wantLimit: 50},
		{name: "honours the cap itself", target: "/v1/transactions?limit=100", wantStatus: http.StatusOK, wantLimit: 100},
		{name: "caps a limit above the maximum", target: "/v1/transactions?limit=10000", wantStatus: http.StatusOK, wantLimit: 100},
		{name: "falls back to the default on zero", target: "/v1/transactions?limit=0", wantStatus: http.StatusOK, wantLimit: 20},
		{name: "rejects a negative limit", target: "/v1/transactions?limit=-1", wantStatus: http.StatusBadRequest, wantCode: "invalid_limit"},
		{name: "rejects a limit that is not a number", target: "/v1/transactions?limit=many", wantStatus: http.StatusBadRequest, wantCode: "invalid_limit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service := mocks.NewTransactionService(t)
			if tt.wantStatus == http.StatusOK {
				service.EXPECT().
					List(mock.Anything, domain.ListParams{
						Query: domain.ListQuery{Sort: domain.SortCreatedAtDesc},
						Limit: tt.wantLimit,
					}).
					Return(domain.KeysetPage{}, nil).
					Once()
			}

			recorder := get(t, newTestServer(service, idlePinger(t)), tt.target)

			assert.Equal(t, tt.wantStatus, recorder.Code)
			if tt.wantStatus == http.StatusOK {
				return
			}
			assert.Equal(t, tt.wantCode, decodeBody[errorResponse](t, recorder).Error.Code)
		})
	}
}

func TestList_ValidatesTheFilterParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		target     string
		wantStatus int
		wantCode   string
		wantQuery  domain.ListQuery
		wantToken  string
	}{
		{
			name:       "no parameter at all means the unfiltered first page, newest first",
			target:     "/v1/transactions",
			wantStatus: http.StatusOK,
			wantQuery:  domain.ListQuery{Sort: domain.SortCreatedAtDesc},
		},
		{
			name:       "carries every filter and the cursor through",
			target:     "/v1/transactions?account_id=42&status=SETTLED&sort=created_at:asc&cursor=abc",
			wantStatus: http.StatusOK,
			wantQuery:  domain.ListQuery{AccountID: 42, Status: "SETTLED", Sort: domain.SortCreatedAtAsc},
			wantToken:  "abc",
		},
		{name: "rejects an unknown sort", target: "/v1/transactions?sort=amount:desc", wantStatus: http.StatusBadRequest, wantCode: "invalid_sort"},
		{name: "rejects a mixed sort", target: "/v1/transactions?sort=created_at:desc,id:asc", wantStatus: http.StatusBadRequest, wantCode: "invalid_sort"},
		{name: "rejects an account_id that is not a number", target: "/v1/transactions?account_id=abc", wantStatus: http.StatusBadRequest, wantCode: "invalid_account_id"},
		{name: "rejects a negative account_id", target: "/v1/transactions?account_id=-1", wantStatus: http.StatusBadRequest, wantCode: "invalid_account_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service := mocks.NewTransactionService(t)
			if tt.wantStatus == http.StatusOK {
				service.EXPECT().
					List(mock.Anything, domain.ListParams{
						Query:  tt.wantQuery,
						Limit:  domain.DefaultLimit,
						Cursor: tt.wantToken,
					}).
					Return(domain.KeysetPage{}, nil).
					Once()
			}

			recorder := get(t, newTestServer(service, idlePinger(t)), tt.target)

			assert.Equal(t, tt.wantStatus, recorder.Code)
			if tt.wantStatus == http.StatusOK {
				return
			}
			assert.Equal(t, tt.wantCode, decodeBody[errorResponse](t, recorder).Error.Code)
		})
	}
}

// TestList_SerialisesTheContractedEnvelope pins the wire format: a string id, a
// nullable next, no total anywhere.
func TestList_SerialisesTheContractedEnvelope(t *testing.T) {
	t.Parallel()

	service := mocks.NewTransactionService(t)
	service.EXPECT().
		List(mock.Anything, mock.Anything).
		Return(domain.KeysetPage{
			Transactions: []domain.Transaction{{
				ID:          9500000,
				AccountID:   42,
				AmountCents: 212190,
				Label:       "VIREMENT 09500000",
				CreatedAt:   time.Date(2021, 1, 8, 1, 46, 40, 0, time.UTC),
			}},
			Next:    "XpQF28PoX3CTQl3hKrIqSdr",
			HasMore: true,
		}, nil).
		Once()

	recorder := get(t, newTestServer(service, idlePinger(t)), "/v1/transactions?account_id=42&limit=1")

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{
		"data": [{
			"id": "9500000",
			"account_id": 42,
			"amount_cents": 212190,
			"label": "VIREMENT 09500000",
			"created_at": "2021-01-08T01:46:40Z"
		}],
		"page": {"next": "XpQF28PoX3CTQl3hKrIqSdr", "has_more": true}
	}`, recorder.Body.String())
}

func TestList_ReportsTheEndOfTheWalkWithANullNext(t *testing.T) {
	t.Parallel()

	service := mocks.NewTransactionService(t)
	service.EXPECT().
		List(mock.Anything, mock.Anything).
		Return(domain.KeysetPage{Transactions: nil, HasMore: false}, nil).
		Once()

	recorder := get(t, newTestServer(service, idlePinger(t)), "/v1/transactions")

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"data": [], "page": {"next": null, "has_more": false}}`, recorder.Body.String())
}

func TestList_MapsCursorFailuresToTheContractedStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "a forged or unreadable cursor", err: cursor.ErrInvalidCursor, wantStatus: http.StatusBadRequest, wantCode: "invalid_cursor"},
		{name: "a cursor past its TTL", err: cursor.ErrCursorExpired, wantStatus: http.StatusGone, wantCode: "cursor_expired"},
		{name: "a cursor replayed on other filters", err: cursor.ErrFilterMismatch, wantStatus: http.StatusBadRequest, wantCode: "cursor_filter_mismatch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service := mocks.NewTransactionService(t)
			service.EXPECT().
				List(mock.Anything, domain.ListParams{
					Query:  domain.ListQuery{Sort: domain.SortCreatedAtDesc},
					Limit:  domain.DefaultLimit,
					Cursor: "whatever",
				}).
				Return(domain.KeysetPage{}, tt.err).
				Once()

			recorder := get(t, newTestServer(service, idlePinger(t)), "/v1/transactions?cursor=whatever")

			assert.Equal(t, tt.wantStatus, recorder.Code)
			assert.Equal(t, tt.wantCode, decodeBody[errorResponse](t, recorder).Error.Code)
		})
	}
}

func TestList_NeverLeaksTheInternalErrorOnA500(t *testing.T) {
	t.Parallel()

	service := mocks.NewTransactionService(t)
	service.EXPECT().
		List(mock.Anything, mock.Anything).
		Return(domain.KeysetPage{}, errors.New(`pq: relation "transactions" does not exist`)).
		Once()

	recorder := get(t, newTestServer(service, idlePinger(t)), "/v1/transactions")

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "transactions")
	assert.JSONEq(t, `{"error":{"code":"internal_error","message":"internal server error"}}`, recorder.Body.String())
}

func TestListByOffset_ValidatesThePageParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		target     string
		wantStatus int
		wantCode   string
		wantPage   int
		wantSize   int
	}{
		{name: "defaults to the first page of twenty", target: "/v1/transactions/offset", wantStatus: http.StatusOK, wantPage: 1, wantSize: 20},
		{name: "reads page and size", target: "/v1/transactions/offset?page=3&size=50", wantStatus: http.StatusOK, wantPage: 3, wantSize: 50},
		{name: "caps the size like the keyset limit", target: "/v1/transactions/offset?size=10000", wantStatus: http.StatusOK, wantPage: 1, wantSize: 100},
		{name: "rejects page zero", target: "/v1/transactions/offset?page=0", wantStatus: http.StatusBadRequest, wantCode: "invalid_page"},
		{name: "rejects a negative page", target: "/v1/transactions/offset?page=-2", wantStatus: http.StatusBadRequest, wantCode: "invalid_page"},
		{name: "rejects a page that is not a number", target: "/v1/transactions/offset?page=first", wantStatus: http.StatusBadRequest, wantCode: "invalid_page"},
		{name: "rejects a size that is not a number", target: "/v1/transactions/offset?size=many", wantStatus: http.StatusBadRequest, wantCode: "invalid_limit"},
		{name: "rejects an account_id that is not a number", target: "/v1/transactions/offset?account_id=abc", wantStatus: http.StatusBadRequest, wantCode: "invalid_account_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service := mocks.NewTransactionService(t)
			if tt.wantStatus == http.StatusOK {
				service.EXPECT().
					ListByOffset(mock.Anything, domain.OffsetParams{Page: tt.wantPage, Size: tt.wantSize}).
					Return(domain.OffsetPage{}, nil).
					Once()
			}

			recorder := get(t, newTestServer(service, idlePinger(t)), tt.target)

			assert.Equal(t, tt.wantStatus, recorder.Code)
			if tt.wantStatus == http.StatusOK {
				return
			}
			assert.Equal(t, tt.wantCode, decodeBody[errorResponse](t, recorder).Error.Code)
		})
	}
}

func TestTotal_AnswersAnEstimateDependingExactParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		queryParam         string
		transactionService func(t *testing.T) *mocks.TransactionService
		status             int
		bodyResponse       string
	}{
		{
			name:       "should return total with no param fallback to estimation",
			queryParam: "",
			transactionService: func(t *testing.T) *mocks.TransactionService {
				service := mocks.NewTransactionService(t)
				service.EXPECT().Total(mock.Anything, false).Return(10_000_000, nil).Once()
				return service
			},
			status:       http.StatusOK,
			bodyResponse: `{"estimate": 10000000, "exact": false}`,
		},
		{
			name:       "should return total with param fallback to estimation",
			queryParam: "exact=false",
			transactionService: func(t *testing.T) *mocks.TransactionService {
				service := mocks.NewTransactionService(t)
				service.EXPECT().Total(mock.Anything, false).Return(10_000_000, nil).Once()
				return service
			},
			status:       http.StatusOK,
			bodyResponse: `{"estimate": 10000000, "exact": false}`,
		},
		{
			name:       "should return total with exact count",
			queryParam: "exact=true",
			transactionService: func(t *testing.T) *mocks.TransactionService {
				service := mocks.NewTransactionService(t)
				service.EXPECT().Total(mock.Anything, true).Return(10_000_000, nil).Once()
				return service
			},
			status:       http.StatusOK,
			bodyResponse: `{"estimate": 10000000, "exact": true}`,
		},
		{
			name:       "should return total with estimation when exact is not true or false",
			queryParam: "exact=toto",
			transactionService: func(t *testing.T) *mocks.TransactionService {
				service := mocks.NewTransactionService(t)
				service.EXPECT().Total(mock.Anything, false).Return(10_000_000, nil).Once()
				return service
			},
			status:       http.StatusOK,
			bodyResponse: `{"estimate": 10000000, "exact": false}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service := tc.transactionService(t)
			recorder := get(t, newTestServer(service, idlePinger(t)), "/v1/transactions/total?"+tc.queryParam)

			require.Equal(t, tc.status, recorder.Code)
			assert.JSONEq(t, tc.bodyResponse, recorder.Body.String())
		})
	}
}

func TestHealth_FollowsThePool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
		wantBody   string
	}{
		{name: "ok while the pool answers", wantStatus: http.StatusOK, wantBody: `{"status":"ok"}`},
		{
			name:       "unavailable once it does not",
			pingErr:    errors.New("connection refused"),
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"unavailable"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := mocks.NewPinger(t)
			db.EXPECT().Ping(mock.Anything).Return(tt.pingErr).Once()

			recorder := get(t, newTestServer(mocks.NewTransactionService(t), db), "/healthz")

			assert.Equal(t, tt.wantStatus, recorder.Code)
			assert.JSONEq(t, tt.wantBody, recorder.Body.String())
		})
	}
}
