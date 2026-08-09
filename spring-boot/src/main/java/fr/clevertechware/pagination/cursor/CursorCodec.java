package fr.clevertechware.pagination.cursor;

import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;

import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import java.nio.charset.StandardCharsets;
import java.security.GeneralSecurityException;
import java.security.MessageDigest;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.LocalDateTime;
import java.time.OffsetDateTime;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import java.time.format.DateTimeParseException;
import java.util.Arrays;
import java.util.Base64;

/**
 * Turns a {@link Cursor} into a signed opaque token and back.
 *
 * <p>{@code token = base64url_nopad( HMAC_SHA256(key, payload) || payload )}, the 32 MAC bytes
 * first. The payload is canonical JSON: fixed key order, no whitespace, {@code created_at} in
 * RFC 3339 UTC with exactly six decimals. Both implementations of the contract produce the same
 * bytes, which is what makes a token issued by one accepted by the other.
 */
public class CursorCodec {

    private static final String HMAC_ALGORITHM = "HmacSHA256";
    private static final int SIGNATURE_LENGTH = 32;
    private static final DateTimeFormatter PAYLOAD_TIMESTAMP =
            DateTimeFormatter.ofPattern("uuuu-MM-dd'T'HH:mm:ss.SSSSSS'Z'");
    private static final Base64.Encoder BASE64URL_ENCODER = Base64.getUrlEncoder().withoutPadding();
    private static final Base64.Decoder BASE64URL_DECODER = Base64.getUrlDecoder();
    private static final ObjectMapper JSON = new ObjectMapper();

    private final byte[] key;
    private final Duration timeToLive;
    private final Clock clock;

    public CursorCodec(byte[] key, Duration timeToLive, Clock clock) {
        this.key = key.clone();
        this.timeToLive = timeToLive;
        this.clock = clock;
    }

    public Cursor issue(OffsetDateTime createdAt, long id, boolean descending, String fingerprint) {
        return new Cursor(
                Cursor.CURRENT_VERSION, createdAt, id, descending, fingerprint, clock.instant().getEpochSecond());
    }

    public String encode(Cursor cursor) {
        byte[] payload = canonicalPayload(cursor).getBytes(StandardCharsets.UTF_8);
        byte[] signature = sign(payload);
        byte[] token = new byte[signature.length + payload.length];
        System.arraycopy(signature, 0, token, 0, signature.length);
        System.arraycopy(payload, 0, token, signature.length, payload.length);
        return BASE64URL_ENCODER.encodeToString(token);
    }

    public Cursor decode(String token) {
        byte[] raw = decodeBase64(token);
        if (raw.length <= SIGNATURE_LENGTH) {
            throw new InvalidCursorException("cursor is too short to carry a signature");
        }
        byte[] signature = Arrays.copyOf(raw, SIGNATURE_LENGTH);
        byte[] payload = Arrays.copyOfRange(raw, SIGNATURE_LENGTH, raw.length);
        if (!MessageDigest.isEqual(signature, sign(payload))) {
            throw new InvalidCursorException("cursor signature does not verify");
        }
        Cursor cursor = readPayload(payload);
        if (cursor.version() != Cursor.CURRENT_VERSION) {
            throw new InvalidCursorException("unsupported cursor version " + cursor.version());
        }
        rejectIfExpired(cursor);
        return cursor;
    }

    /**
     * The canonical payload, written by hand: neither the key order nor the timestamp format of a
     * general-purpose serializer is part of its contract, and both are part of ours. The only two
     * string values are a base64url fingerprint and a formatted timestamp, so no JSON escaping can
     * ever apply.
     */
    private static String canonicalPayload(Cursor cursor) {
        return new StringBuilder(128)
                .append("{\"v\":").append(cursor.version())
                .append(",\"c\":\"").append(PAYLOAD_TIMESTAMP.format(cursor.createdAt().withOffsetSameInstant(ZoneOffset.UTC)))
                .append("\",\"i\":").append(cursor.id())
                .append(",\"d\":").append(cursor.descending())
                .append(",\"f\":\"").append(cursor.fingerprint())
                .append("\",\"t\":").append(cursor.issuedAt())
                .append('}')
                .toString();
    }

    private Cursor readPayload(byte[] payload) {
        CanonicalPayload parsed;
        try {
            parsed = JSON.readValue(payload, CanonicalPayload.class);
        } catch (Exception e) {
            throw new InvalidCursorException("cursor payload is not readable");
        }
        if (parsed.createdAt() == null || parsed.fingerprint() == null) {
            throw new InvalidCursorException("cursor payload is missing a required field");
        }
        return new Cursor(
                parsed.version(),
                parseTimestamp(parsed.createdAt()),
                parsed.id(),
                parsed.descending(),
                parsed.fingerprint(),
                parsed.issuedAt());
    }

    private static OffsetDateTime parseTimestamp(String value) {
        try {
            return OffsetDateTime.of(LocalDateTime.parse(value, PAYLOAD_TIMESTAMP), ZoneOffset.UTC);
        } catch (DateTimeParseException e) {
            throw new InvalidCursorException("cursor timestamp is not RFC 3339 UTC with six decimals");
        }
    }

    private void rejectIfExpired(Cursor cursor) {
        Instant expiry = Instant.ofEpochSecond(cursor.issuedAt()).plus(timeToLive);
        if (clock.instant().isAfter(expiry)) {
            throw new CursorExpiredException("cursor was issued more than " + timeToLive + " ago");
        }
    }

    private static byte[] decodeBase64(String token) {
        try {
            return BASE64URL_DECODER.decode(token);
        } catch (IllegalArgumentException e) {
            throw new InvalidCursorException("cursor is not valid base64url");
        }
    }

    private byte[] sign(byte[] payload) {
        try {
            Mac mac = Mac.getInstance(HMAC_ALGORITHM);
            mac.init(new SecretKeySpec(key, HMAC_ALGORITHM));
            return mac.doFinal(payload);
        } catch (GeneralSecurityException e) {
            throw new IllegalStateException("HMAC-SHA256 is required by every JRE", e);
        }
    }

    private record CanonicalPayload(
            @JsonProperty("v") int version,
            @JsonProperty("c") String createdAt,
            @JsonProperty("i") long id,
            @JsonProperty("d") boolean descending,
            @JsonProperty("f") String fingerprint,
            @JsonProperty("t") long issuedAt) {
    }
}
