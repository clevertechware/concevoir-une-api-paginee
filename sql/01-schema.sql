-- Schema of the article "Concevoir une API paginée qui tient la charge".
-- Applied once by the PostgreSQL entrypoint when the volume is created.

CREATE TABLE transactions (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  account_id   bigint NOT NULL,
  amount_cents bigint NOT NULL,
  label        text   NOT NULL,
  created_at   timestamptz NOT NULL
);

-- Matches ORDER BY created_at DESC, id DESC exactly: the unfiltered keyset walk
-- descends this B-tree instead of counting rows from the top.
CREATE INDEX idx_txn_created_id ON transactions (created_at DESC, id DESC);

-- The tenant filter comes first, then the sort key, then the tie-breaker.
-- Any other column order turns the cursor bound into a Filter.
CREATE INDEX idx_txn_acct ON transactions (account_id, created_at DESC, id DESC);
