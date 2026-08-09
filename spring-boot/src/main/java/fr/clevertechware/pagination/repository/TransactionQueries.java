package fr.clevertechware.pagination.repository;

import fr.clevertechware.pagination.domain.SortOrder;

/**
 * Every statement the application sends to PostgreSQL, spelled out.
 *
 * <p>The first page and the following ones are <b>two distinct statements</b>. Folding them into one
 * with a {@code (? IS NULL OR (created_at, id) < (?, ?))} guard looks tidy and reads the same, but the
 * day the driver switches to a generic plan the bound stops being an {@code Index Cond} and becomes a
 * {@code Filter}: the database walks down from the top of the partition again, which is {@code OFFSET}
 * wearing keyset syntax. {@code ExecutionPlanIT} pins both behaviours.
 *
 * <p>Placeholders are positional so the exact same string can be handed to {@code EXPLAIN} by the
 * tests: what the plan tests measure is the statement the application really runs.
 */
final class TransactionQueries {

    private static final String COLUMNS = "id, account_id, amount_cents, label, created_at";

    private TransactionQueries() {
    }

    /** Parameters: limit. */
    static String firstPage(SortOrder sort) {
        return """
                SELECT %s
                FROM transactions
                ORDER BY created_at %s, id %s
                LIMIT ?""".formatted(COLUMNS, sort.sqlDirection(), sort.sqlDirection());
    }

    /** Parameters: cursor created_at, cursor id, limit. */
    static String nextPage(SortOrder sort) {
        return """
                SELECT %s
                FROM transactions
                WHERE (created_at, id) %s (?, ?)
                ORDER BY created_at %s, id %s
                LIMIT ?""".formatted(COLUMNS, sort.sqlBoundOperator(), sort.sqlDirection(), sort.sqlDirection());
    }

    /** Parameters: account id, limit. */
    static String firstPageForAccount(SortOrder sort) {
        return """
                SELECT %s
                FROM transactions
                WHERE account_id = ?
                ORDER BY created_at %s, id %s
                LIMIT ?""".formatted(COLUMNS, sort.sqlDirection(), sort.sqlDirection());
    }

    /** Parameters: account id, cursor created_at, cursor id, limit. */
    static String nextPageForAccount(SortOrder sort) {
        return """
                SELECT %s
                FROM transactions
                WHERE account_id = ?
                  AND (created_at, id) %s (?, ?)
                ORDER BY created_at %s, id %s
                LIMIT ?""".formatted(COLUMNS, sort.sqlBoundOperator(), sort.sqlDirection(), sort.sqlDirection());
    }

    /** Parameters: limit, offset. */
    static final String OFFSET_PAGE = """
            SELECT %s
            FROM transactions
            ORDER BY created_at DESC, id DESC
            LIMIT ? OFFSET ?""".formatted(COLUMNS);

    /** Parameters: account id, limit, offset. */
    static final String OFFSET_PAGE_FOR_ACCOUNT = """
            SELECT %s
            FROM transactions
            WHERE account_id = ?
            ORDER BY created_at DESC, id DESC
            LIMIT ? OFFSET ?""".formatted(COLUMNS);

    /**
     * Parameters: after id, limit.
     *
     * <p>No first-page variant here: the walk starts at {@code id > 0} and the primary key is an
     * identity column, so the beginning of the export is a real bound rather than a null to guard.
     */
    static final String EXPORT_PAGE = """
            SELECT %s
            FROM transactions
            WHERE id > ?
            ORDER BY id
            LIMIT ?""".formatted(COLUMNS);

    /** Planner statistics rather than a count: {@code reltuples} is -1 until the table is analysed. */
    static final String ESTIMATED_ROW_COUNT =
            "SELECT reltuples::bigint FROM pg_class WHERE oid = 'transactions'::regclass";

    static final String PING = "SELECT 1";
}
