package handler

import (
	"strconv"

	"github.com/gin-gonic/gin/binding"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/domain"
)

// Each endpoint binds its query string into one request struct, and each
// parameter is a named type that validates itself.
//
// The rules do not live in `binding` struct tags because the validator only
// runs once the string has been converted: a non-numeric limit fails before it,
// with an error naming neither the field nor its rule. The contract answers
// those with the failing parameter's own code, and UnmarshalParam is the only
// hook that still knows which parameter it is reading.
var (
	_ binding.BindUnmarshaler = (*accountID)(nil)
	_ binding.BindUnmarshaler = (*pageSize)(nil)
	_ binding.BindUnmarshaler = (*pageNumber)(nil)
	_ binding.BindUnmarshaler = (*sortOrder)(nil)
	_ binding.BindUnmarshaler = (*exactFlag)(nil)
)

// keysetListRequest is the query string of GET /v1/transactions.
type keysetListRequest struct {
	AccountID accountID `form:"account_id"`
	Status    string    `form:"status"`
	Sort      sortOrder `form:"sort"`
	Limit     pageSize  `form:"limit"`
	Cursor    string    `form:"cursor"`
}

func (r keysetListRequest) params() domain.ListParams {
	return domain.ListParams{
		Query:  domain.ListQuery{AccountID: r.AccountID.value(), Status: r.Status, Sort: r.Sort.value()},
		Limit:  r.Limit.value(),
		Cursor: r.Cursor,
	}
}

// offsetListRequest is the query string of GET /v1/transactions/offset.
type offsetListRequest struct {
	AccountID accountID  `form:"account_id"`
	Page      pageNumber `form:"page"`
	Size      pageSize   `form:"size"`
}

func (r offsetListRequest) params() domain.OffsetParams {
	return domain.OffsetParams{AccountID: r.AccountID.value(), Page: r.Page.value(), Size: r.Size.value()}
}

// totalRequest is the query string of GET /v1/transactions/total.
type totalRequest struct {
	Exact exactFlag `form:"exact"`
}

// accountID is the positive bigint tenant filter. Absent is 0, which is also
// the value the cursor fingerprint uses for "no filter".
type accountID int64

func (a *accountID) UnmarshalParam(raw string) error {
	if raw == "" {
		return nil
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return domain.ErrInvalidAccountID
	}

	*a = accountID(value)
	return nil
}

func (a accountID) value() int64 { return int64(a) }

// pageSize is the limit of the keyset endpoint and the size of the offset one.
// The contract holds them to the same rule, down to the error code.
type pageSize int

func (s *pageSize) UnmarshalParam(raw string) error {
	if raw == "" {
		return nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return domain.ErrInvalidLimit
	}

	*s = pageSize(value)
	return nil
}

// value caps rather than rejects: a client asking for 10 000 rows gets 100 and
// a 200. Absent and zero both fall back to the default.
func (s pageSize) value() int {
	return domain.CapLimit(int(s), domain.DefaultLimit, domain.MaxLimit)
}

// pageNumber is the 1-based page of the offset endpoint.
type pageNumber int

// UnmarshalParam parses a raw string into a pageNumber, ensuring it is a valid integer greater than or equal to 1.
func (n *pageNumber) UnmarshalParam(raw string) error {
	if raw == "" {
		return nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return domain.ErrInvalidPage
	}

	*n = pageNumber(value)
	return nil
}

// value returns the 1-based page number, defaulting to 1 if absent.
func (n pageNumber) value() int {
	if n == 0 {
		return 1
	}
	return int(n)
}

// sortOrder is the pagination order, and it defers to domain.ParseSort rather
// than repeating the two accepted values here.
type sortOrder domain.Sort

func (o *sortOrder) UnmarshalParam(raw string) error {
	sort, err := domain.ParseSort(raw)
	if err != nil {
		return err
	}

	*o = sortOrder(sort)
	return nil
}

func (o sortOrder) value() domain.Sort {
	if o == "" {
		return domain.SortCreatedAtDesc
	}
	return domain.Sort(o)
}

// exactFlag is deliberately permissive: only "true" asks for the expensive
// count, and anything else falls back to the estimate rather than to a 400.
type exactFlag bool

func (f *exactFlag) UnmarshalParam(raw string) error {
	*f = exactFlag(raw == "true")
	return nil
}

func (f exactFlag) value() bool { return bool(f) }
