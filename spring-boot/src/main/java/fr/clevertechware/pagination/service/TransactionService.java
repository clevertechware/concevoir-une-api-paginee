package fr.clevertechware.pagination.service;

import fr.clevertechware.pagination.cursor.Cursor;
import fr.clevertechware.pagination.cursor.CursorCodec;
import fr.clevertechware.pagination.cursor.FilterFingerprint;
import fr.clevertechware.pagination.cursor.InvalidCursorException;
import fr.clevertechware.pagination.domain.CursorFilterMismatchException;
import fr.clevertechware.pagination.domain.ExportPage;
import fr.clevertechware.pagination.domain.ExportQuery;
import fr.clevertechware.pagination.domain.KeysetPage;
import fr.clevertechware.pagination.domain.ListQuery;
import fr.clevertechware.pagination.domain.OffsetPage;
import fr.clevertechware.pagination.domain.OffsetQuery;
import fr.clevertechware.pagination.domain.Transaction;
import fr.clevertechware.pagination.repository.TransactionRepository;
import org.springframework.stereotype.Service;

import java.util.List;

@Service
public class TransactionService {

    private final TransactionRepository repository;
    private final CursorCodec cursorCodec;

    public TransactionService(TransactionRepository repository, CursorCodec cursorCodec) {
        this.repository = repository;
        this.cursorCodec = cursorCodec;
    }

    public KeysetPage list(ListQuery query) {
        String fingerprint = FilterFingerprint.of(query.accountId(), query.status(), query.sort());
        Cursor cursor = query.cursorToken() == null ? null : decodeAgainst(query, fingerprint);

        List<Transaction> fetched = fetchOneMoreThanThePage(query, cursor);
        boolean hasMore = fetched.size() > query.limit();
        List<Transaction> rows = hasMore ? fetched.subList(0, query.limit()) : fetched;

        return new KeysetPage(rows, hasMore ? nextCursorAfter(rows, query, fingerprint) : null, hasMore);
    }

    public OffsetPage listByOffset(OffsetQuery query) {
        int fetchSize = query.size() + 1;
        List<Transaction> fetched = query.hasAccountFilter()
                ? repository.findByOffsetForAccount(query.accountId(), fetchSize, query.offset())
                : repository.findByOffset(fetchSize, query.offset());

        boolean hasMore = fetched.size() > query.size();
        List<Transaction> rows = hasMore ? fetched.subList(0, query.size()) : fetched;

        return new OffsetPage(rows, query.page(), query.size(), hasMore);
    }

    public ExportPage export(ExportQuery query) {
        List<Transaction> fetched = repository.findAfterId(query.afterId(), query.limit() + 1);
        boolean hasMore = fetched.size() > query.limit();
        List<Transaction> rows = hasMore ? fetched.subList(0, query.limit()) : fetched;

        Long nextAfterId = rows.isEmpty() ? null : rows.getLast().id();
        return new ExportPage(rows, hasMore ? nextAfterId : null, hasMore);
    }

    public long estimatedRowCount() {
        return repository.estimatedRowCount();
    }

    public boolean isDatabaseReachable() {
        return repository.isReachable();
    }

    private Cursor decodeAgainst(ListQuery query, String fingerprint) {
        Cursor cursor = cursorCodec.decode(query.cursorToken());
        if (!cursor.fingerprint().equals(fingerprint)) {
            throw new CursorFilterMismatchException("cursor was issued for another set of filters");
        }
        if (cursor.descending() != query.sort().descending()) {
            throw new InvalidCursorException("cursor direction contradicts its own fingerprint");
        }
        return cursor;
    }

    /**
     * The row beyond the page is what answers {@code has_more}. It costs one row, against the 101 ms
     * of a {@code COUNT(*)}, and it is trimmed here so it never reaches the client.
     */
    private List<Transaction> fetchOneMoreThanThePage(ListQuery query, Cursor cursor) {
        int limit = query.limit() + 1;
        if (cursor == null) {
            return query.hasAccountFilter()
                    ? repository.findFirstPageForAccount(query.accountId(), query.sort(), limit)
                    : repository.findFirstPage(query.sort(), limit);
        }
        return query.hasAccountFilter()
                ? repository.findAfterCursorForAccount(
                        query.accountId(), cursor.createdAt(), cursor.id(), query.sort(), limit)
                : repository.findAfterCursor(cursor.createdAt(), cursor.id(), query.sort(), limit);
    }

    private String nextCursorAfter(List<Transaction> rows, ListQuery query, String fingerprint) {
        Transaction last = rows.getLast();
        return cursorCodec.encode(
                cursorCodec.issue(last.createdAt(), last.id(), query.sort().descending(), fingerprint));
    }
}
