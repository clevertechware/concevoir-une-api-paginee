package fr.clevertechware.pagination.domain;

/**
 * @param accountId   tenant filter, {@link #NO_ACCOUNT_FILTER} when absent — the same convention the
 *                    fingerprint uses, so query and cursor agree without a null to special-case
 * @param status      reserved by the contract: it does not reach SQL yet, but it is already part of
 *                    the fingerprint, so adding it later cannot silently accept stale cursors
 * @param sort        normalised sort order
 * @param limit       page size already capped by the web layer
 * @param cursorToken opaque token, {@code null} on the first page
 */
public record ListQuery(long accountId, String status, SortOrder sort, int limit, String cursorToken) {

    public static final long NO_ACCOUNT_FILTER = 0L;

    public boolean hasAccountFilter() {
        return accountId != NO_ACCOUNT_FILTER;
    }
}
