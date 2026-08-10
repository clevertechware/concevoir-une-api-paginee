package service

import (
	"context"
	"time"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/config"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/domain"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/cursor"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/logger"
)

// Transactions serves the four listing endpoints. It owns the cursor: the
// repository never sees a token, and the handler never sees a bound.
type Transactions struct {
	repository transactionRepository
	logger     logger.Logger
	cursorKey  []byte
	cursorTTL  time.Duration
	// now stamps the tokens this service issues and decides whether an incoming
	// one has expired. Injected so the expiry test does not have to wait
	// three days, or trust the wall clock to stand still mid-test.
	now func() time.Time
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
func (s *Transactions) List(
	ctx context.Context, q domain.ListQuery, limit int, token string,
) (domain.KeysetPage, error) {
	fingerprint := cursor.Fingerprint(cursor.Filters{
		AccountID: q.AccountID,
		Status:    q.Status,
		Sort:      string(q.Sort),
	})

	// One row more than the page. That extra row is the entire cost of knowing
	// whether a next page exists, and it never leaves the server.
	fetch := limit + 1

	var (
		rows []domain.Transaction
		err  error
	)
	if token == "" {
		rows, err = s.repository.FirstPage(ctx, q, fetch)
	} else {
		var position cursor.Cursor
		position, err = cursor.DecodeAt(token, s.cursorKey, s.cursorTTL, s.now())
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

	rows, hasMore := trim(rows, limit)

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
	ctx context.Context, accountID int64, page, size int,
) (domain.OffsetPage, error) {
	rows, err := s.repository.OffsetPage(ctx, accountID, (page-1)*size, size+1)
	if err != nil {
		return domain.OffsetPage{}, err
	}

	rows, hasMore := trim(rows, size)

	return domain.OffsetPage{Transactions: rows, Page: page, Size: size, HasMore: hasMore}, nil
}

// Export returns one slice of the full walk. No signed cursor here: the
// position is a public primary key, so there is nothing to hide and nothing a
// client could forge that it could not already ask for outright.
func (s *Transactions) Export(
	ctx context.Context, afterID int64, limit int,
) (domain.ExportPage, error) {
	rows, err := s.repository.Export(ctx, afterID, limit+1)
	if err != nil {
		return domain.ExportPage{}, err
	}

	rows, hasMore := trim(rows, limit)

	result := domain.ExportPage{Transactions: rows, HasMore: hasMore}
	if hasMore {
		result.NextAfterID = rows[len(rows)-1].ID
	}
	return result, nil
}

// CountEstimate returns the planner's row estimate, assumed as an estimate.
func (s *Transactions) CountEstimate(ctx context.Context) (int64, error) {
	return s.repository.CountEstimate(ctx)
}

// trim drops the extra row fetched to answer has_more.
func trim(rows []domain.Transaction, limit int) ([]domain.Transaction, bool) {
	if len(rows) > limit {
		return rows[:limit], true
	}
	return rows, false
}
