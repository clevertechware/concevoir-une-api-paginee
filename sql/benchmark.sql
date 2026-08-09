-- The measurements quoted in the article, in the order they appear.
-- Assumes a 10 000 000 row seed:
--
--   make db-up seed ROWS=10000000
--   make bench
--
-- The absolute timings depend on the machine; the ratios between depths do not.

\timing on
\echo
\echo '=== OFFSET, depth 0 ==============================================='
EXPLAIN (ANALYZE, BUFFERS, COSTS OFF, TIMING OFF)
SELECT id, account_id, amount_cents, created_at
FROM transactions
ORDER BY created_at DESC, id DESC
LIMIT 20 OFFSET 0;

\echo
\echo '=== OFFSET, depth 500 000 ========================================='
EXPLAIN (ANALYZE, BUFFERS, COSTS OFF, TIMING OFF)
SELECT id, account_id, amount_cents, created_at
FROM transactions
ORDER BY created_at DESC, id DESC
LIMIT 20 OFFSET 500000;

\echo
\echo '=== OFFSET, depth 5 000 000 ======================================='
\echo '--- actual rows under the Limit is the number to read here ---'
EXPLAIN (ANALYZE, BUFFERS, COSTS OFF, TIMING OFF)
SELECT id, account_id, amount_cents, created_at
FROM transactions
ORDER BY created_at DESC, id DESC
LIMIT 20 OFFSET 5000000;

\echo
\echo '=== KEYSET, depth 500 000 ========================================='
\echo '--- the bound must appear under Index Cond, never under Filter ---'
EXPLAIN (ANALYZE, BUFFERS, COSTS OFF, TIMING OFF)
SELECT id, account_id, amount_cents, created_at
FROM transactions
WHERE (created_at, id) < (timestamptz '2021-01-08 01:47:00+00', 9500001)
ORDER BY created_at DESC, id DESC
LIMIT 20;

\echo
\echo '=== KEYSET, depth 5 000 000 ======================================='
EXPLAIN (ANALYZE, BUFFERS, COSTS OFF, TIMING OFF)
SELECT id, account_id, amount_cents, created_at
FROM transactions
WHERE (created_at, id) < (timestamptz '2018-03-03 09:47:00+00', 5000001)
ORDER BY created_at DESC, id DESC
LIMIT 20;

\echo
\echo '=== COUNT(*), the total nobody should pay for ====================='
EXPLAIN (ANALYZE, BUFFERS, COSTS OFF, TIMING OFF)
SELECT count(*) FROM transactions;

\echo
\echo '=== The "one SQL for every page" trap ============================='
\echo '--- custom plan: the IS NULL is simplified away, the bound holds ---'
EXPLAIN (ANALYZE, BUFFERS, COSTS OFF, TIMING OFF)
SELECT id, account_id, amount_cents, created_at
FROM transactions
WHERE account_id = 42
  AND (timestamptz '2018-03-03 09:47:00+00' IS NULL
       OR (created_at, id) < (timestamptz '2018-03-03 09:47:00+00', 5000001))
ORDER BY created_at DESC, id DESC
LIMIT 20;

\echo
\echo '--- generic plan: the same predicate degrades into a Filter ---'
PREPARE paged (timestamptz, bigint) AS
SELECT id, account_id, amount_cents, created_at
FROM transactions
WHERE account_id = 42
  AND ($1 IS NULL OR (created_at, id) < ($1, $2))
ORDER BY created_at DESC, id DESC
LIMIT 20;

SET plan_cache_mode = force_generic_plan;
EXPLAIN (ANALYZE, BUFFERS, COSTS OFF, TIMING OFF)
EXECUTE paged (timestamptz '2018-03-03 09:47:00+00', 5000001);
RESET plan_cache_mode;
DEALLOCATE paged;
