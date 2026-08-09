package fr.clevertechware.pagination.domain;

/**
 * The two orderings the API accepts, and nothing else.
 *
 * <p>Both columns always move in the same direction: a mixed sort would break the row comparison
 * {@code (created_at, id) < (?, ?)}, which only has one meaning for the whole tuple. That is the
 * reason this enum has two values rather than a grammar.
 */
public enum SortOrder {

    CREATED_AT_DESC("created_at:desc", "DESC", "<"),
    CREATED_AT_ASC("created_at:asc", "ASC", ">");

    private final String value;
    private final String sqlDirection;
    private final String sqlBoundOperator;

    SortOrder(String value, String sqlDirection, String sqlBoundOperator) {
        this.value = value;
        this.sqlDirection = sqlDirection;
        this.sqlBoundOperator = sqlBoundOperator;
    }

    public String value() {
        return value;
    }

    public String sqlDirection() {
        return sqlDirection;
    }

    public String sqlBoundOperator() {
        return sqlBoundOperator;
    }

    public boolean descending() {
        return this == CREATED_AT_DESC;
    }

    public static SortOrder fromValue(String value) {
        for (SortOrder order : values()) {
            if (order.value.equals(value)) {
                return order;
            }
        }
        throw new IllegalArgumentException("unknown sort " + value);
    }
}
