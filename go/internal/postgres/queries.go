package postgres

import "github.com/clevertechware/concevoir-une-api-paginee-golang/internal/domain"

// This file is the one to read alongside the article. Every statement the demo
// issues is spelled out here, in full, rather than assembled by a builder: the
// shape of the SQL *is* the argument.

const transactionColumns = `id, account_id, amount_cents, label, created_at`

// keysetQueries holds the four statements one sort direction needs.
//
// Four, not one. The tempting single statement —
//
//	WHERE account_id = $1 AND ($2::timestamptz IS NULL OR (created_at, id) < ($2, $3))
//
// works right up to the point where the driver stops re-planning and switches
// to a generic plan. From then on $2 is unknown at planning time, the predicate
// can no longer position the index scan, and it degrades into a Filter: the
// database walks down from the top of the partition and throws rows away one by
// one. That is OFFSET, written in keyset syntax.
//
// The cost of avoiding it is this struct. TestExplain_TheSingleQueryTrap proves
// the trap is real, and TestExplain_NextPageBoundIsAnIndexCond proves these
// statements avoid it.
type keysetQueries struct {
	firstPage          string
	nextPage           string
	firstPageByAccount string
	nextPageByAccount  string
}

// descendingQueries walks newest first. The bound is `<` and both ORDER BY
// columns descend — always in the same direction, because a tuple comparison
// has one meaning for the whole tuple. A mixed order would need the predicate
// expanded by hand and an index built to match, which is exactly why the sort
// parameter only accepts two values.
var descendingQueries = keysetQueries{
	firstPage: `
		SELECT ` + transactionColumns + `
		FROM transactions
		ORDER BY created_at DESC, id DESC
		LIMIT $1`,

	nextPage: `
		SELECT ` + transactionColumns + `
		FROM transactions
		WHERE (created_at, id) < ($1, $2)
		ORDER BY created_at DESC, id DESC
		LIMIT $3`,

	firstPageByAccount: `
		SELECT ` + transactionColumns + `
		FROM transactions
		WHERE account_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2`,

	nextPageByAccount: `
		SELECT ` + transactionColumns + `
		FROM transactions
		WHERE account_id = $1
		  AND (created_at, id) < ($2, $3)
		ORDER BY created_at DESC, id DESC
		LIMIT $4`,
}

// ascendingQueries walks the same index the other way: the bound flips to `>`
// and both ORDER BY columns ascend.
var ascendingQueries = keysetQueries{
	firstPage: `
		SELECT ` + transactionColumns + `
		FROM transactions
		ORDER BY created_at ASC, id ASC
		LIMIT $1`,

	nextPage: `
		SELECT ` + transactionColumns + `
		FROM transactions
		WHERE (created_at, id) > ($1, $2)
		ORDER BY created_at ASC, id ASC
		LIMIT $3`,

	firstPageByAccount: `
		SELECT ` + transactionColumns + `
		FROM transactions
		WHERE account_id = $1
		ORDER BY created_at ASC, id ASC
		LIMIT $2`,

	nextPageByAccount: `
		SELECT ` + transactionColumns + `
		FROM transactions
		WHERE account_id = $1
		  AND (created_at, id) > ($2, $3)
		ORDER BY created_at ASC, id ASC
		LIMIT $4`,
}

func queriesFor(sort domain.Sort) keysetQueries {
	if sort.Descending() {
		return descendingQueries
	}
	return ascendingQueries
}

// The counter-example. Same data, same order, and the database has to count
// past every skipped row before it can return anything. Kept so the demo can
// measure the two side by side, not so anyone reuses it.
const (
	offsetPage = `
		SELECT ` + transactionColumns + `
		FROM transactions
		ORDER BY created_at DESC, id DESC
		LIMIT $1 OFFSET $2`

	offsetPageByAccount = `
		SELECT ` + transactionColumns + `
		FROM transactions
		WHERE account_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3`
)

// countQuery is the exact count that countEstimate exists to avoid.
const countQuery = `SELECT count(*) FROM transactions`

// countEstimate reads the planner's own row estimate instead of counting.
// The COUNT(*) it replaces takes 101 ms and 93 457 blocks on the article's
// dataset, against 0,025 ms for the page it would accompany.
const countEstimate = `SELECT reltuples::bigint FROM pg_class WHERE oid = 'transactions'::regclass`
