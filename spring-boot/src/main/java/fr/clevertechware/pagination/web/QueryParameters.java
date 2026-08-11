package fr.clevertechware.pagination.web;

import fr.clevertechware.pagination.domain.ListQuery;
import fr.clevertechware.pagination.domain.SortOrder;

/**
 * Query parameters are read as raw strings so each one can fail with the code the contract names
 * for it, rather than with whatever a generic type converter would have produced.
 */
final class QueryParameters {

    private QueryParameters() {
    }

    static long accountId(String raw) {
        if (raw == null || raw.isBlank()) {
            return ListQuery.NO_ACCOUNT_FILTER;
        }
        long accountId = parseLong(raw, "invalid_account_id", "account_id must be a positive integer");
        if (accountId <= 0) {
            throw new InvalidParameterException("invalid_account_id", "account_id must be a positive integer");
        }
        return accountId;
    }

    static String status(String raw) {
        return raw == null ? "" : raw;
    }

    static SortOrder sort(String raw) {
        if (raw == null || raw.isBlank()) {
            return SortOrder.CREATED_AT_DESC;
        }
        try {
            return SortOrder.fromValue(raw);
        } catch (IllegalArgumentException e) {
            throw new InvalidParameterException(
                    "invalid_sort", "sort must be created_at:desc or created_at:asc");
        }
    }

    /** Capping rather than rejecting is the contract: an oversized limit is served, not refused. */
    static int limit(String raw, int defaultValue, int maximum) {
        return cappedSize(raw, defaultValue, maximum, "invalid_limit", "limit");
    }

    static int size(String raw, int defaultValue, int maximum) {
        return cappedSize(raw, defaultValue, maximum, "invalid_limit", "size");
    }

    static int page(String raw) {
        if (raw == null || raw.isBlank()) {
            return 1;
        }
        int page = (int) parseLong(raw, "invalid_page", "page must be an integer greater than or equal to 1");
        if (page < 1) {
            throw new InvalidParameterException(
                    "invalid_page", "page must be an integer greater than or equal to 1");
        }
        return page;
    }

    static String cursor(String raw) {
        return raw == null || raw.isBlank() ? null : raw;
    }

    private static int cappedSize(String raw, int defaultValue, int maximum, String code, String name) {
        if (raw == null || raw.isBlank()) {
            return defaultValue;
        }
        long requested = parseLong(raw, code, name + " must be a positive integer");
        // AIP-158 reads an explicit zero as "no preference", not as an error. Only a negative
        // value is a client bug worth a 400.
        if (requested == 0) {
            return defaultValue;
        }
        if (requested < 0) {
            throw new InvalidParameterException(code, name + " must be a positive integer");
        }
        return (int) Math.min(requested, maximum);
    }

    private static long parseLong(String raw, String code, String message) {
        try {
            return Long.parseLong(raw.trim());
        } catch (NumberFormatException e) {
            throw new InvalidParameterException(code, message);
        }
    }
}
