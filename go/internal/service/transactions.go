package service

import (
	"context"
	"time"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/config"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/domain"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/cursor"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/logger"
)

// Transactions serves the three listing endpoints. It owns the cursor: the
// repository never sees a token, and the handler never sees a bound.
type Transactions struct {
	repository transactionRepository
	logger     logger.Logger
	cursorKey  []byte
	cursorTTL  time.Duration
	now        func() time.Time
}

// NewTransactions creates the listing service.
func NewTransactions(
	repository transactionRepository, cfg config.Cursor, log logger.Logger,
) *Transactions {
	return &Transactions{
		repository: repository,
		logger:     log,
		cursorKey:  []byte(cfg.Key),
		cursorTTL:  cfg.TTL,
		now:        time.Now,
	}
}

// List returns one keyset page and the cursor that resumes after it.
//
// An empty token means the first page. Anything else must decode, must not have
// expired, and must carry the fingerprint of the filters being asked for right
// now — otherwise the client changed its query mid-walk and would get a page
// that belongs to neither.
func (s *Transactions) List(ctx context.Context, params domain.ListParams) (domain.KeysetPage, error) {
	q := params.Query
	fingerprint := cursor.Fingerprint(cursor.Filters{
		AccountID: q.AccountID,
		Status:    q.Status,
		Sort:      string(q.Sort),
	})

	fetch := params.Limit + 1
	var (
		rows []domain.Transaction
		err  error
	)

	if params.Cursor == "" {
		rows, err = s.repository.FirstPage(ctx, q, fetch)
	} else {
		var position cursor.Cursor
		position, err = cursor.DecodeAt(params.Cursor, s.cursorKey, s.cursorTTL, s.now())
		if err != nil {
			return domain.KeysetPage{}, err
		}
		if position.Fingerprint != fingerprint {
			return domain.KeysetPage{}, cursor.ErrFilterMismatch
		}
		if position.Descending != q.Sort.Descending() {
			// The fingerprint already covers sort, so this cannot normally
			// trigger. It stays because the field that decides which SQL
			// variant runs deserves a check of its own.
			return domain.KeysetPage{}, cursor.ErrFilterMismatch
		}

		rows, err = s.repository.NextPage(ctx, q, domain.Bound{
			CreatedAt: position.CreatedAt,
			ID:        position.ID,
		}, fetch)
	}
	if err != nil {
		return domain.KeysetPage{}, err
	}

	rows, hasMore := trim(rows, params.Limit)

	page := domain.KeysetPage{Transactions: rows, HasMore: hasMore}
	if hasMore {
		last := rows[len(rows)-1]
		page.Next = cursor.Encode(cursor.Cursor{
			CreatedAt:   last.CreatedAt,
			ID:          last.ID,
			Descending:  q.Sort.Descending(),
			Fingerprint: fingerprint,
			IssuedAt:    s.now(),
		}, s.cursorKey)
	}

	return page, nil
}

// ListByOffset returns one page by rank, the counter-example.
func (s *Transactions) ListByOffset(
	ctx context.Context, params domain.OffsetParams,
) (domain.OffsetPage, error) {
	rows, err := s.repository.OffsetPage(ctx, params.AccountID, (params.Page-1)*params.Size, params.Size+1)
	if err != nil {
		return domain.OffsetPage{}, err
	}

	rows, hasMore := trim(rows, params.Size)

	return domain.OffsetPage{Transactions: rows, Page: params.Page, Size: params.Size, HasMore: hasMore}, nil
}

// Total returns the planner's row estimate, assumed as an estimate.
func (s *Transactions) Total(ctx context.Context, exact bool) (int64, error) {
	if exact {
		return s.repository.Count(ctx)
	}
	return s.repository.CountEstimate(ctx)
}

// trim drops the extra row fetched to answer has_more.
func trim(rows []domain.Transaction, limit int) ([]domain.Transaction, bool) {
	if len(rows) > limit {
		return rows[:limit], true
	}
	return rows, false
}
