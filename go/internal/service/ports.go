// Package service holds the application logic: turning a request into the right
// query, and turning the last row of a page back into a cursor.
package service

import (
	"context"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/domain"
)

// The interface below is declared here, where it is consumed, rather than next
// to its PostgreSQL implementation. The service states what it needs; the
// adapter satisfies it.
//
// FirstPage and NextPage are two methods rather than one method with an
// optional bound, and that separation is the whole point: it is what stops a
// caller from ever asking for "a page, with or without a cursor" and getting
// the single-statement trap the article documents.
type transactionRepository interface {
	FirstPage(ctx context.Context, q domain.ListQuery, limit int) ([]domain.Transaction, error)
	NextPage(ctx context.Context, q domain.ListQuery, after domain.Bound, limit int) ([]domain.Transaction, error)
	OffsetPage(ctx context.Context, accountID int64, offset, limit int) ([]domain.Transaction, error)
	Export(ctx context.Context, afterID int64, limit int) ([]domain.Transaction, error)
	CountEstimate(ctx context.Context) (int64, error)
}
