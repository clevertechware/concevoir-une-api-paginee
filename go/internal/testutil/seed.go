package testutil

import (
	"context"
	"testing"
)

// SeedTransactions empties the table and refills it with rows rows.
//
// Deliberately different from sql/seed.sql: the dataset there spreads rows over
// 2 000 random accounts, which leaves ~50 rows per tenant at the sizes a test
// can afford, far too shallow for the single-query trap to show any damage.
// Here account_id is `1 + (i % accounts)`, so every tenant partition is deep,
// and the mapping stays deterministic: row i carries id i and created_at
// 2015-01-01 + i × 20 s.
func SeedTransactions(t *testing.T, pg *Postgres, rows, accounts int) {
	t.Helper()

	ctx := context.Background()

	if _, err := pg.Pool.Exec(ctx, `TRUNCATE transactions RESTART IDENTITY`); err != nil {
		t.Fatalf("truncating transactions: %v", err)
	}

	const insert = `
		INSERT INTO transactions (account_id, amount_cents, label, created_at)
		SELECT
		  1 + (i % $2),
		  (i * 37) % 500000 - 250000,
		  'VIREMENT ' || to_char(i, 'FM00000000'),
		  timestamptz '2015-01-01 00:00:00+00' + i * interval '20 seconds'
		FROM generate_series(1, $1) AS i`

	if _, err := pg.Pool.Exec(ctx, insert, rows, accounts); err != nil {
		t.Fatalf("seeding transactions: %v", err)
	}

	// Without fresh statistics and a filled visibility map, PostgreSQL can never
	// choose an Index Only Scan, and every plan assertion below would be
	// measuring the wrong thing.
	if _, err := pg.Pool.Exec(ctx, `VACUUM ANALYZE transactions`); err != nil {
		t.Fatalf("analysing transactions: %v", err)
	}
}

// SeedIdenticalTimestamps inserts rows that all share one created_at value.
// This is what the tie-breaker exists for: without id in the sort key and in
// the cursor, the order between these rows is undefined and the walk can return
// one of them twice, or none of them.
func SeedIdenticalTimestamps(t *testing.T, pg *Postgres, rows int) {
	t.Helper()

	ctx := context.Background()

	if _, err := pg.Pool.Exec(ctx, `TRUNCATE transactions RESTART IDENTITY`); err != nil {
		t.Fatalf("truncating transactions: %v", err)
	}

	const insert = `
		INSERT INTO transactions (account_id, amount_cents, label, created_at)
		SELECT 42, i, 'CARTE ' || to_char(i, 'FM00000000'),
		       timestamptz '2020-06-01 12:00:00+00'
		FROM generate_series(1, $1) AS i`

	if _, err := pg.Pool.Exec(ctx, insert, rows); err != nil {
		t.Fatalf("seeding transactions: %v", err)
	}
	if _, err := pg.Pool.Exec(ctx, `VACUUM ANALYZE transactions`); err != nil {
		t.Fatalf("analysing transactions: %v", err)
	}
}
