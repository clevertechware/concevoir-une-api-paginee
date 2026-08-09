-- Reduced dataset for the integration suite: enough rows for a cursor bound to have depth, small
-- enough to load in about a second. The article's own 10M-row seed lives in ../sql/seed.sql and is
-- never used here.
--
-- The account distribution matters as much as the row count. Account 42 holds 2 % of the table, the
-- order of magnitude the article's seed produces with its 2 000 accounts. Spread the rows evenly
-- instead and the planner rightly stops using idx_txn_acct: the tenant filter would become a
-- `Filter` on top of idx_txn_created_id and every plan assertion here would measure the wrong index.

TRUNCATE transactions RESTART IDENTITY;

INSERT INTO transactions (account_id, amount_cents, label, created_at)
SELECT
  CASE WHEN i % 50 = 0 THEN 42 ELSE 100 + (i % 997) END,
  (i * 37) % 500000 - 250000,
  'VIREMENT ' || to_char(i, 'FM00000000'),
  timestamptz '2015-01-01 00:00:00+00' + i * interval '20 seconds'
FROM generate_series(1, 100000) AS i;

-- Fifty rows sharing a single created_at: without the id tie-breaker the order between them is
-- undefined, and a keyset walk over them would drop or repeat rows.
INSERT INTO transactions (account_id, amount_cents, label, created_at)
SELECT 99, i * 100, 'CARTE MEME INSTANT ' || to_char(i, 'FM00'), timestamptz '2016-06-01 12:00:00+00'
FROM generate_series(1, 50) AS i;

VACUUM ANALYZE transactions;
