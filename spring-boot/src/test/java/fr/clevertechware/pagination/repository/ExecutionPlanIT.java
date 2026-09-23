package fr.clevertechware.pagination.repository;

import fr.clevertechware.pagination.domain.SortOrder;
import fr.clevertechware.pagination.support.AbstractDatabaseTest;
import fr.clevertechware.pagination.support.ExecutionPlan;
import org.junit.jupiter.api.Test;

import java.sql.Connection;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.time.OffsetDateTime;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * The claims of the article that only an execution plan can settle.
 *
 * <p>Every statement explained here is the string {@link TransactionQueries} hands to the JDBC
 * client, not a paraphrase of it.
 */
class ExecutionPlanIT extends AbstractDatabaseTest {

    private static final long ACCOUNT_ID = 42L;
    private static final int PAGE_SIZE = 21;

    private record Position(OffsetDateTime createdAt, long id) {

        String asSqlLiterals() {
            return "timestamptz '" + createdAt + "', " + id;
        }
    }

    @Test
    void placesTheCursorBoundUnderIndexCondForTheAccountFilteredNextPage() throws SQLException {
        try (Connection connection = openConnection()) {
            Position position = positionAtDepth(connection, 1000);

            String plan = ExecutionPlan.of(
                    connection,
                    TransactionQueries.nextPageForAccount(SortOrder.CREATED_AT_DESC),
                    ACCOUNT_ID, position.createdAt(), position.id(), PAGE_SIZE);

            assertThat(plan).contains("idx_txn_acct");
            assertThat(plan).contains("Index Cond");
            assertThat(plan).doesNotContain("Rows Removed by Filter");
        }
    }

    @Test
    void placesTheCursorBoundUnderIndexCondForTheUnfilteredNextPage() throws SQLException {
        try (Connection connection = openConnection()) {
            Position position = positionAtDepth(connection, 1000);

            String plan = ExecutionPlan.of(
                    connection,
                    TransactionQueries.nextPage(SortOrder.CREATED_AT_DESC),
                    position.createdAt(), position.id(), PAGE_SIZE);

            assertThat(plan).contains("idx_txn_created_id");
            assertThat(plan).contains("Index Cond");
            assertThat(plan).doesNotContain("Rows Removed by Filter");
        }
    }

    @Test
    void readsTheSameNumberOfBlocksWhateverTheDepthOfTheCursor() throws SQLException {
        try (Connection connection = openConnection()) {
            int nearTheTop = blocksForKeysetPageAt(connection, positionAtDepth(connection, 10));
            int deepDown = blocksForKeysetPageAt(connection, positionAtDepth(connection, 1900));

            assertThat(deepDown).isLessThanOrEqualTo(nearTheTop + 2);
        }
    }

    @Test
    void offsetPaginationReadsMoreAndMoreBlocksAsTheRequestedPageGetsDeeper() throws SQLException {
        try (Connection connection = openConnection()) {
            int firstPage = ExecutionPlan.blocksTouched(
                    ExecutionPlan.of(connection, TransactionQueries.OFFSET_PAGE, 20, 0));
            int deepPage = ExecutionPlan.blocksTouched(
                    ExecutionPlan.of(connection, TransactionQueries.OFFSET_PAGE, 20, 90_000));

            assertThat(deepPage).isGreaterThan(firstPage * 5);
        }
    }

    /**
     * The trap of the article, executed rather than described: one statement for every page, guarded
     * by {@code $1 IS NULL}, run under a generic plan. See {@link TransactionQueries} for why it degrades.
     */
    @Test
    void theSingleStatementGuardedByIsNullDegradesIntoAFilterUnderAGenericPlan() throws SQLException {
        String trap = """
                SELECT id, account_id, amount_cents, label, created_at
                FROM transactions
                WHERE account_id = 42
                  AND ($1 IS NULL OR (created_at, id) < ($1, $2))
                ORDER BY created_at DESC, id DESC
                LIMIT 20""";

        try (Connection connection = openConnection(); Statement session = connection.createStatement()) {
            Position position = positionAtDepth(connection, 1000);
            session.execute("PREPARE trap (timestamptz, bigint) AS " + trap);
            session.execute("SET plan_cache_mode = force_generic_plan");

            String plan = ExecutionPlan.ofStatement(
                    connection, "EXECUTE trap (" + position.asSqlLiterals() + ")");

            assertThat(plan).contains("idx_txn_acct");
            assertThat(plan).contains("Filter:");
            assertThat(plan).contains("Rows Removed by Filter");
        }
    }

    @Test
    void theTwoStatementVariantKeepsItsIndexBoundUnderTheSameGenericPlan() throws SQLException {
        String nextPage = ExecutionPlan.withNumberedPlaceholders(
                TransactionQueries.nextPageForAccount(SortOrder.CREATED_AT_DESC));

        try (Connection connection = openConnection(); Statement session = connection.createStatement()) {
            Position position = positionAtDepth(connection, 1000);
            session.execute("PREPARE keyset (bigint, timestamptz, bigint, int) AS " + nextPage);
            session.execute("SET plan_cache_mode = force_generic_plan");

            String plan = ExecutionPlan.ofStatement(
                    connection,
                    "EXECUTE keyset (" + ACCOUNT_ID + ", " + position.asSqlLiterals() + ", " + PAGE_SIZE + ")");

            assertThat(plan).contains("idx_txn_acct");
            assertThat(plan).contains("Index Cond");
            assertThat(plan).doesNotContain("Rows Removed by Filter");
        }
    }

    /** Explained twice: the first run is only there to warm the cache, so the two depths are comparable. */
    private static int blocksForKeysetPageAt(Connection connection, Position position) throws SQLException {
        String sql = TransactionQueries.nextPageForAccount(SortOrder.CREATED_AT_DESC);
        Object[] parameters = {ACCOUNT_ID, position.createdAt(), position.id(), PAGE_SIZE};
        ExecutionPlan.of(connection, sql, parameters);
        return ExecutionPlan.blocksTouched(ExecutionPlan.of(connection, sql, parameters));
    }

    private static Position positionAtDepth(Connection connection, int depth) throws SQLException {
        String sql = """
                SELECT created_at, id
                FROM transactions
                WHERE account_id = %d
                ORDER BY created_at DESC, id DESC
                OFFSET %d LIMIT 1""".formatted(ACCOUNT_ID, depth);
        try (Statement statement = connection.createStatement(); ResultSet row = statement.executeQuery(sql)) {
            row.next();
            return new Position(row.getObject("created_at", OffsetDateTime.class), row.getLong("id"));
        }
    }
}
