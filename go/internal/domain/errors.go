package domain

import "errors"

// Request validation failures. Each one maps to a 400 and to the error code of
// the same name in spec/contract.md §2.
//
// The cursor failures are not here: they belong to pkg/cursor, which is the
// public contract package and cannot depend on internal packages. The handler
// maps both families in one place.
var (
	// ErrInvalidLimit indicates a limit or size parameter that is negative or not a number.
	ErrInvalidLimit = errors.New("limit must be a non-negative integer")
	// ErrInvalidPage indicates a page parameter below 1 or not a number.
	ErrInvalidPage = errors.New("page must be an integer greater than or equal to 1")
	// ErrInvalidSort indicates a sort parameter outside the two accepted values.
	ErrInvalidSort = errors.New("sort must be created_at:desc or created_at:asc")
	// ErrInvalidAccountID indicates an account_id parameter that is not a positive integer.
	ErrInvalidAccountID = errors.New("account_id must be a positive integer")
	// ErrInvalidAfterID indicates an after_id parameter that is not a positive integer.
	ErrInvalidAfterID = errors.New("after_id must be a positive integer")
)
