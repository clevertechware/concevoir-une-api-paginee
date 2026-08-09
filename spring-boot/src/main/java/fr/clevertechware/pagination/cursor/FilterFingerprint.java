package fr.clevertechware.pagination.cursor;

import fr.clevertechware.pagination.domain.SortOrder;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.Arrays;
import java.util.Base64;

/**
 * Checksum of the filters a cursor was issued for.
 *
 * <p>Normative form, shared with the Go implementation:
 * {@code base64url_nopad(SHA256("<account_id>|<status>|<sort>")[0:8])}, where an absent
 * account filter is the literal {@code 0} and an absent status the empty string.
 */
public final class FilterFingerprint {

    private static final Base64.Encoder BASE64URL = Base64.getUrlEncoder().withoutPadding();
    private static final int FINGERPRINT_BYTES = 8;

    private FilterFingerprint() {
    }

    public static String of(long accountId, String status, SortOrder sort) {
        String canonical = accountId + "|" + status + "|" + sort.value();
        byte[] digest = sha256(canonical.getBytes(StandardCharsets.UTF_8));
        return BASE64URL.encodeToString(Arrays.copyOf(digest, FINGERPRINT_BYTES));
    }

    private static byte[] sha256(byte[] input) {
        try {
            return MessageDigest.getInstance("SHA-256").digest(input);
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalStateException("SHA-256 is required by every JRE", e);
        }
    }
}
