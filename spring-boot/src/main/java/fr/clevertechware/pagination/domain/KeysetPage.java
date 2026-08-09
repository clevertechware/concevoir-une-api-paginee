package fr.clevertechware.pagination.domain;

import java.util.List;

/**
 * @param rows       exactly the requested page size at most; the extra row fetched for
 *                   {@code hasMore} never leaves the server
 * @param nextCursor {@code null} at the end of the walk, and the only authority on that end
 */
public record KeysetPage(List<Transaction> rows, String nextCursor, boolean hasMore) {
}
