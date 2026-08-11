package fr.clevertechware.pagination.repository;

import fr.clevertechware.pagination.domain.SortOrder;
import fr.clevertechware.pagination.domain.Transaction;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.simple.JdbcClient;
import org.springframework.stereotype.Repository;

import java.time.OffsetDateTime;
import java.util.List;

/**
 * Read-only access to the transactions table.
 *
 * <p>One method per statement, on purpose: a generic query builder would hide which of the four
 * keyset variants is running, and which variant runs is exactly what decides the execution plan.
 */
@Repository
public class TransactionRepository {

    private static final RowMapper<Transaction> TRANSACTION_MAPPER = (rs, rowNum) -> new Transaction(
            rs.getLong("id"),
            rs.getLong("account_id"),
            rs.getLong("amount_cents"),
            rs.getString("label"),
            rs.getObject("created_at", OffsetDateTime.class));

    private final JdbcClient jdbcClient;

    public TransactionRepository(JdbcClient jdbcClient) {
        this.jdbcClient = jdbcClient;
    }

    public List<Transaction> findFirstPage(SortOrder sort, int limit) {
        return jdbcClient.sql(TransactionQueries.firstPage(sort))
                .param(limit)
                .query(TRANSACTION_MAPPER)
                .list();
    }

    public List<Transaction> findAfterCursor(OffsetDateTime createdAt, long id, SortOrder sort, int limit) {
        return jdbcClient.sql(TransactionQueries.nextPage(sort))
                .param(createdAt)
                .param(id)
                .param(limit)
                .query(TRANSACTION_MAPPER)
                .list();
    }

    public List<Transaction> findFirstPageForAccount(long accountId, SortOrder sort, int limit) {
        return jdbcClient.sql(TransactionQueries.firstPageForAccount(sort))
                .param(accountId)
                .param(limit)
                .query(TRANSACTION_MAPPER)
                .list();
    }

    public List<Transaction> findAfterCursorForAccount(
            long accountId, OffsetDateTime createdAt, long id, SortOrder sort, int limit) {
        return jdbcClient.sql(TransactionQueries.nextPageForAccount(sort))
                .param(accountId)
                .param(createdAt)
                .param(id)
                .param(limit)
                .query(TRANSACTION_MAPPER)
                .list();
    }

    public List<Transaction> findByOffset(int limit, int offset) {
        return jdbcClient.sql(TransactionQueries.OFFSET_PAGE)
                .param(limit)
                .param(offset)
                .query(TRANSACTION_MAPPER)
                .list();
    }

    public List<Transaction> findByOffsetForAccount(long accountId, int limit, int offset) {
        return jdbcClient.sql(TransactionQueries.OFFSET_PAGE_FOR_ACCOUNT)
                .param(accountId)
                .param(limit)
                .param(offset)
                .query(TRANSACTION_MAPPER)
                .list();
    }

    public long estimatedRowCount() {
        Long estimate = jdbcClient.sql(TransactionQueries.ESTIMATED_ROW_COUNT)
                .query(Long.class)
                .optional()
                .orElse(0L);
        return Math.max(estimate, 0L);
    }

    public boolean isReachable() {
        try {
            jdbcClient.sql(TransactionQueries.PING).query(Integer.class).single();
            return true;
        } catch (RuntimeException e) {
            return false;
        }
    }
}
