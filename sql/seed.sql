-- Seeds the transactions table exactly as the article does.
--
--   psql -v rows=10000000 -f sql/seed.sql
--
-- setseed is called before the first random(), so the same PostgreSQL version
-- replaying this script produces the same accounts and the same amounts.

\if :{?rows}
\else
  \set rows 10000000
\endif

TRUNCATE transactions RESTART IDENTITY;

SELECT setseed(0.42);

INSERT INTO transactions (account_id, amount_cents, label, created_at)
SELECT
  1 + floor(random() * 2000)::bigint,
  floor(random() * 500000)::bigint - 250000,
  (ARRAY['CARTE','VIREMENT','PRELEVEMENT','CHEQUE','RETRAIT'])[1 + floor(random() * 5)::int]
    || ' ' || to_char(i, 'FM00000000'),
  timestamptz '2015-01-01 00:00:00+00' + i * interval '20 seconds'
FROM generate_series(1, :rows) AS i;

-- Refreshes the planner statistics and fills the visibility map, without which
-- PostgreSQL can never choose an Index Only Scan.
VACUUM ANALYZE transactions;
