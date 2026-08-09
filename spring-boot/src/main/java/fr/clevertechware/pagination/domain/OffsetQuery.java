package fr.clevertechware.pagination.domain;

/** The counter-example of the article: pagination by rank, kept only to be measured against keyset. */
public record OffsetQuery(long accountId, int page, int size) {

    public boolean hasAccountFilter() {
        return accountId != ListQuery.NO_ACCOUNT_FILTER;
    }

    public int offset() {
        return (page - 1) * size;
    }
}
