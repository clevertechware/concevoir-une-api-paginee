// Package domain holds the entities and the vocabulary of the demo. It depends
// on nothing else in the project.
package domain

import "time"

// Transaction is one row of the table the article paginates.
type Transaction struct {
	ID          int64
	AccountID   int64
	AmountCents int64
	Label       string
	CreatedAt   time.Time
}

// Sort is the pagination order. Only two values exist, and that is a design
// decision rather than a shortcut: a mixed order such as created_at DESC, id ASC
// would break the tuple comparison the keyset bound relies on.
type Sort string

const (
	// SortCreatedAtDesc is the default: newest first.
	SortCreatedAtDesc Sort = "created_at:desc"
	// SortCreatedAtAsc walks the same index the other way.
	SortCreatedAtAsc Sort = "created_at:asc"
)

// ParseSort normalises a sort parameter, defaulting to descending when absent.
func ParseSort(raw string) (Sort, error) {
	switch Sort(raw) {
	case "":
		return SortCreatedAtDesc, nil
	case SortCreatedAtDesc:
		return SortCreatedAtDesc, nil
	case SortCreatedAtAsc:
		return SortCreatedAtAsc, nil
	default:
		return "", ErrInvalidSort
	}
}

// Descending reports whether the walk goes from the newest row to the oldest.
func (s Sort) Descending() bool { return s != SortCreatedAtAsc }

// ListQuery is everything a keyset walk is bound to. The cursor fingerprint
// covers exactly these fields, so changing any of them mid-walk is detected
// rather than silently answered with an inconsistent page.
//
// AccountID is 0 when no tenant filter is applied, which is also the value the
// fingerprint formula uses for an absent filter.
type ListQuery struct {
	AccountID int64
	Status    string
	Sort      Sort
}

// Bound is the position a keyset page resumes from.
type Bound struct {
	CreatedAt time.Time
	ID        int64
}

// KeysetPage is one page of the recommended endpoint. Next is empty at the end
// of the walk, and it is the only authority on that.
type KeysetPage struct {
	Transactions []Transaction
	Next         string
	HasMore      bool
}

// OffsetPage is one page of the counter-example endpoint.
type OffsetPage struct {
	Transactions []Transaction
	Page         int
	Size         int
	HasMore      bool
}
