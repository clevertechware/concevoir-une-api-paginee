package fr.clevertechware.pagination.service;

import fr.clevertechware.pagination.domain.KeysetPage;
import fr.clevertechware.pagination.domain.ListQuery;
import fr.clevertechware.pagination.domain.OffsetPage;
import fr.clevertechware.pagination.domain.OffsetQuery;
import fr.clevertechware.pagination.domain.SortOrder;
import fr.clevertechware.pagination.domain.Transaction;
import fr.clevertechware.pagination.support.AbstractDatabaseTest;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;

import java.sql.Connection;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * Drift is the claim of the article that has nothing to do with speed: on a dataset that moves
 * during the walk, {@code OFFSET} hands the same row twice and skips others without ever raising an
 * error, while a keyset walk does not.
 *
 * <p>Both walks below run against the same table, with the same interleaved insertions.
 */
class KeysetDriftIT extends AbstractDatabaseTest {

    private static final long WALKED_ACCOUNT = 900L;
    private static final int ORIGINAL_ROWS = 60;
    private static final int PAGE_SIZE = 10;
    private static final int ROWS_INSERTED_AT_THE_HEAD = 5;

    @Autowired
    private TransactionService service;

    @BeforeEach
    void seedTheWalkedAccount() {
        execute("DELETE FROM transactions WHERE account_id = " + WALKED_ACCOUNT);
        execute("""
                INSERT INTO transactions (account_id, amount_cents, label, created_at)
                SELECT %d, i * 10, 'ORIGINAL ' || to_char(i, 'FM000'),
                       timestamptz '2020-01-01 00:00:00+00' + i * interval '1 minute'
                FROM generate_series(1, %d) AS i
                """.formatted(WALKED_ACCOUNT, ORIGINAL_ROWS));
    }

    @AfterEach
    void deleteTheWalkedAccount() {
        execute("DELETE FROM transactions WHERE account_id = " + WALKED_ACCOUNT);
    }

    @Test
    void aKeysetWalkInterruptedByInsertionsAtTheHeadReturnsEveryOriginalRowExactlyOnce() {
        Set<Long> originalIds = idsOfTheWalkedAccount();
        List<Long> seen = new ArrayList<>();

        String cursor = null;
        boolean firstPage = true;
        do {
            KeysetPage page = service.list(keysetQuery(cursor));
            page.rows().stream().map(Transaction::id).forEach(seen::add);
            cursor = page.nextCursor();
            if (firstPage) {
                insertNewerRowsAtTheHead();
                firstPage = false;
            }
        } while (cursor != null);

        assertThat(seen).doesNotHaveDuplicates();
        assertThat(seen).containsAll(originalIds);
    }

    @Test
    void anOffsetWalkInterruptedByTheSameInsertionsHandsBackRowsItAlreadyReturned() {
        List<Long> seen = new ArrayList<>();

        int page = 1;
        boolean firstPage = true;
        boolean hasMore;
        do {
            OffsetPage current = service.listByOffset(new OffsetQuery(WALKED_ACCOUNT, page, PAGE_SIZE));
            current.rows().stream().map(Transaction::id).forEach(seen::add);
            hasMore = current.hasMore();
            page++;
            if (firstPage) {
                insertNewerRowsAtTheHead();
                firstPage = false;
            }
        } while (hasMore);

        assertThat(seen).hasSizeGreaterThan(new HashSet<>(seen).size());
    }

    @Test
    void anOffsetWalkInterruptedByDeletionsAtTheHeadSkipsRowsItNeverReturned() {
        Set<Long> originalIds = idsOfTheWalkedAccount();
        List<Long> seen = new ArrayList<>();

        int page = 1;
        boolean firstPage = true;
        boolean hasMore;
        do {
            OffsetPage current = service.listByOffset(new OffsetQuery(WALKED_ACCOUNT, page, PAGE_SIZE));
            current.rows().stream().map(Transaction::id).forEach(seen::add);
            hasMore = current.hasMore();
            page++;
            if (firstPage) {
                deleteNewestRows();
                firstPage = false;
            }
        } while (hasMore);

        assertThat(missing(originalIds, seen)).isNotEmpty();
    }

    @Test
    void aKeysetWalkOverRowsSharingASingleTimestampStaysCompleteThanksToTheTieBreaker() {
        Set<Long> expected = idsOf(99L);
        List<Long> seen = new ArrayList<>();

        String cursor = null;
        do {
            KeysetPage page = service.list(new ListQuery(99L, "", SortOrder.CREATED_AT_DESC, 7, cursor));
            page.rows().stream().map(Transaction::id).forEach(seen::add);
            cursor = page.nextCursor();
        } while (cursor != null);

        assertThat(seen).doesNotHaveDuplicates();
        assertThat(seen).containsExactlyInAnyOrderElementsOf(expected);
    }

    @Test
    void anAscendingKeysetWalkVisitsTheSameRowsAsTheDescendingOne() {
        List<Long> descending = walkFully(SortOrder.CREATED_AT_DESC);
        List<Long> ascending = walkFully(SortOrder.CREATED_AT_ASC);

        assertThat(ascending).doesNotHaveDuplicates();
        assertThat(ascending).containsExactlyInAnyOrderElementsOf(descending);
    }

    private List<Long> walkFully(SortOrder sort) {
        List<Long> seen = new ArrayList<>();
        String cursor = null;
        do {
            KeysetPage page = service.list(new ListQuery(WALKED_ACCOUNT, "", sort, PAGE_SIZE, cursor));
            page.rows().stream().map(Transaction::id).forEach(seen::add);
            cursor = page.nextCursor();
        } while (cursor != null);
        return seen;
    }

    private ListQuery keysetQuery(String cursor) {
        return new ListQuery(WALKED_ACCOUNT, "", SortOrder.CREATED_AT_DESC, PAGE_SIZE, cursor);
    }

    private void insertNewerRowsAtTheHead() {
        execute("""
                INSERT INTO transactions (account_id, amount_cents, label, created_at)
                SELECT %d, i, 'INSERTED DURING THE WALK ' || i,
                       timestamptz '2021-01-01 00:00:00+00' + i * interval '1 minute'
                FROM generate_series(1, %d) AS i
                """.formatted(WALKED_ACCOUNT, ROWS_INSERTED_AT_THE_HEAD));
    }

    private void deleteNewestRows() {
        execute("""
                DELETE FROM transactions
                WHERE id IN (
                  SELECT id FROM transactions
                  WHERE account_id = %d
                  ORDER BY created_at DESC, id DESC
                  LIMIT %d)
                """.formatted(WALKED_ACCOUNT, ROWS_INSERTED_AT_THE_HEAD));
    }

    private Set<Long> idsOfTheWalkedAccount() {
        return idsOf(WALKED_ACCOUNT);
    }

    private Set<Long> idsOf(long accountId) {
        Set<Long> ids = new HashSet<>();
        try (Connection connection = openConnection();
             Statement statement = connection.createStatement();
             ResultSet rows = statement.executeQuery(
                     "SELECT id FROM transactions WHERE account_id = " + accountId)) {
            while (rows.next()) {
                ids.add(rows.getLong("id"));
            }
        } catch (SQLException e) {
            throw new IllegalStateException(e);
        }
        return ids;
    }

    private static Set<Long> missing(Set<Long> expected, List<Long> seen) {
        Set<Long> missing = new HashSet<>(expected);
        seen.forEach(missing::remove);
        return missing;
    }
}
