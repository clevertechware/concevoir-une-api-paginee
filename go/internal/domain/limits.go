package domain

// Page size boundaries, fixed by spec/contract.md rather than configurable:
// they are part of the contract, and a deployment that could change them would
// make the contract untrue.
const (
	// DefaultLimit and MaxLimit apply to the keyset and offset endpoints.
	DefaultLimit = 20
	MaxLimit     = 100
)

// CapLimit clamps a page size instead of rejecting it. A client that asks for
// 10 000 rows gets 100 and a 200, not a 400: the request is answerable, and
// refusing it only teaches the client to retry.
func CapLimit(requested, fallback, maxAllowed int) int {
	switch {
	case requested == 0:
		return fallback
	case requested > maxAllowed:
		return maxAllowed
	default:
		return requested
	}
}
