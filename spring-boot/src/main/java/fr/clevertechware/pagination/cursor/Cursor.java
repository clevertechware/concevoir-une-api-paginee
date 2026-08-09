package fr.clevertechware.pagination.cursor;

import java.time.OffsetDateTime;

/**
 * The position of a keyset walk, as carried across requests by an opaque token.
 *
 * @param version    format version, so the payload can change without breaking tokens in flight
 * @param createdAt  sort key of the last row handed to the client
 * @param id         tie-breaker of that same row
 * @param descending direction of the walk, so a token cannot be replayed backwards
 * @param fingerprint checksum of the filters the token was issued for
 * @param issuedAt   emission time, Unix seconds, the input of the expiry check
 */
public record Cursor(
        int version,
        OffsetDateTime createdAt,
        long id,
        boolean descending,
        String fingerprint,
        long issuedAt) {

    public static final int CURRENT_VERSION = 1;
}
