package postgres

import (
	"context"
	"fmt"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/logger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/domain"
)

// TransactionRepository reads transactions. Read-only by design: the schema is
// shared infrastructure.
//
// Every method takes the limit it should pass to the database verbatim. The
// caller is the one that asks for one row more than the page and drops it, so
// that the extra row answering has_more is visible where the decision is made.
type TransactionRepository struct {
	pool   *pgxpool.Pool
	logger logger.Logger
}

// NewTransactionRepository creates a TransactionRepository.
func NewTransactionRepository(pool *pgxpool.Pool, log logger.Logger) *TransactionRepository {
	return &TransactionRepository{pool: pool, logger: log}
}

// FirstPage returns the head of a keyset walk. There is no cursor, so there is no
// bound at all, rather than a bound neutralised by an IS NULL.
func (r *TransactionRepository) FirstPage(
	ctx context.Context, q domain.ListQuery, limit int,
) ([]domain.Transaction, error) {
	queries := queriesFor(q.Sort)

	if q.AccountID > 0 {
		return r.query(ctx, queries.firstPageByAccount, q.AccountID, limit)
	}
	return r.query(ctx, queries.firstPage, limit)
}

// NextPage resumes a keyset walk from the position the cursor carries. The
// tuple comparison is what lets PostgreSQL descend the B-tree straight to that
// position instead of counting rows from the top.
func (r *TransactionRepository) NextPage(
	ctx context.Context, q domain.ListQuery, after domain.Bound, limit int,
) ([]domain.Transaction, error) {
	queries := queriesFor(q.Sort)

	if q.AccountID > 0 {
		return r.query(ctx, queries.nextPageByAccount, q.AccountID, after.CreatedAt, after.ID, limit)
	}
	return r.query(ctx, queries.nextPage, after.CreatedAt, after.ID, limit)
}

// OffsetPage returns a page by rank. Present to be measured against the keyset
// walk, not to be recommended.
func (r *TransactionRepository) OffsetPage(
	ctx context.Context, accountID int64, offset, limit int,
) ([]domain.Transaction, error) {
	if accountID > 0 {
		return r.query(ctx, offsetPageByAccount, accountID, limit, offset)
	}
	return r.query(ctx, offsetPage, limit, offset)
}

// Count retrieves the total number of rows in the `transactions` table. It returns the count and an error, if any.
func (r *TransactionRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.pool.QueryRow(ctx, countQuery).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting rows: %w", err)
	}
	return count, nil
}

// CountEstimate returns the planner's row estimate for the table.
//
// reltuples is -1 on a table that has never been analysed, which means "no
// estimate" rather than "no rows"; report 0 rather than a negative count.
func (r *TransactionRepository) CountEstimate(ctx context.Context) (int64, error) {
	var estimate int64
	if err := r.pool.QueryRow(ctx, countEstimate).Scan(&estimate); err != nil {
		return 0, fmt.Errorf("estimating row count: %w", err)
	}
	if estimate < 0 {
		return 0, nil
	}
	return estimate, nil
}

// Ping reports whether the pool can still reach the database.
func (r *TransactionRepository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

func (r *TransactionRepository) query(
	ctx context.Context, sql string, args ...any,
) ([]domain.Transaction, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("selecting transactions: %w", err)
	}
	defer rows.Close()

	transactions := make([]domain.Transaction, 0)
	for rows.Next() {
		transaction, err := scanTransaction(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning transaction: %w", err)
		}
		transactions = append(transactions, transaction)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating transactions: %w", err)
	}

	return transactions, nil
}

func scanTransaction(row pgx.Row) (domain.Transaction, error) {
	var t domain.Transaction
	err := row.Scan(&t.ID, &t.AccountID, &t.AmountCents, &t.Label, &t.CreatedAt)
	return t, err
}
