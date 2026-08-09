package fr.clevertechware.pagination.cursor;

import fr.clevertechware.pagination.domain.SortOrder;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.OffsetDateTime;
import java.time.ZoneOffset;
import java.util.Base64;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatExceptionOfType;

class CursorCodecTest {

    private static final byte[] KEY = "concevoir-une-api-paginee-test-key".getBytes(StandardCharsets.UTF_8);
    private static final Duration TTL = Duration.ofHours(72);
    private static final Instant NOW = Instant.parse("2026-01-01T00:00:00Z");
    private static final OffsetDateTime POSITION = OffsetDateTime.parse("2021-05-03T19:32:40.123456Z");

    private final CursorCodec codec = new CursorCodec(KEY, TTL, Clock.fixed(NOW, ZoneOffset.UTC));

    @Test
    void roundTripsAPositionThroughItsToken() {
        Cursor issued = codec.issue(POSITION, 9_999_998L, true, "jlkDlANVKaQ");

        assertThat(codec.decode(codec.encode(issued))).isEqualTo(issued);
    }

    @Test
    void rejectsATokenWhoseSingleFlippedByteBreaksTheSignature() {
        String token = codec.encode(codec.issue(POSITION, 42L, true, "jlkDlANVKaQ"));

        assertThatExceptionOfType(InvalidCursorException.class)
                .isThrownBy(() -> codec.decode(flipLastPayloadByte(token)));
    }

    @Test
    void rejectsATokenSignedWithAnotherKey() {
        CursorCodec otherService = new CursorCodec(
                "another-key".getBytes(StandardCharsets.UTF_8), TTL, Clock.fixed(NOW, ZoneOffset.UTC));
        String foreignToken = otherService.encode(otherService.issue(POSITION, 42L, true, "jlkDlANVKaQ"));

        assertThatExceptionOfType(InvalidCursorException.class).isThrownBy(() -> codec.decode(foreignToken));
    }

    @Test
    void rejectsATokenThatIsNotBase64Url() {
        assertThatExceptionOfType(InvalidCursorException.class).isThrownBy(() -> codec.decode("not a cursor!!"));
    }

    @Test
    void rejectsATokenTooShortToCarryASignature() {
        String tooShort = Base64.getUrlEncoder().withoutPadding().encodeToString(new byte[16]);

        assertThatExceptionOfType(InvalidCursorException.class).isThrownBy(() -> codec.decode(tooShort));
    }

    @Test
    void rejectsAProperlySignedTokenWhoseFormatVersionIsUnknown() {
        Cursor fromTheFuture = new Cursor(2, POSITION, 42L, true, "jlkDlANVKaQ", NOW.getEpochSecond());
        String token = codec.encode(fromTheFuture);

        assertThatExceptionOfType(InvalidCursorException.class).isThrownBy(() -> codec.decode(token));
    }

    @Test
    void reportsAnExpiredCursorApartFromAnInvalidOneSoTheClientCanRestartTheWalk() {
        String token = codec.encode(codec.issue(POSITION, 42L, true, "jlkDlANVKaQ"));
        CursorCodec threeDaysLater = new CursorCodec(
                KEY, TTL, Clock.fixed(NOW.plus(Duration.ofHours(72)).plusSeconds(1), ZoneOffset.UTC));

        assertThatExceptionOfType(CursorExpiredException.class).isThrownBy(() -> threeDaysLater.decode(token));
    }

    @Test
    void acceptsACursorIssuedJustInsideTheRetentionWindow() {
        String token = codec.encode(codec.issue(POSITION, 42L, true, "jlkDlANVKaQ"));
        CursorCodec almostThreeDaysLater = new CursorCodec(
                KEY, TTL, Clock.fixed(NOW.plus(Duration.ofHours(72)), ZoneOffset.UTC));

        assertThat(almostThreeDaysLater.decode(token).id()).isEqualTo(42L);
    }

    @Test
    void keepsMicrosecondPrecisionOfTheSortKeyAcrossTheRoundTrip() {
        Cursor issued = codec.issue(POSITION, 42L, true, "jlkDlANVKaQ");

        assertThat(codec.decode(codec.encode(issued)).createdAt().toInstant())
                .isEqualTo(POSITION.toInstant());
    }

    @Test
    void bindsTheFingerprintOfTheFiltersItWasIssuedFor() {
        String fingerprint = FilterFingerprint.of(42L, "", SortOrder.CREATED_AT_DESC);
        Cursor issued = codec.issue(POSITION, 42L, true, fingerprint);

        assertThat(codec.decode(codec.encode(issued)).fingerprint())
                .isEqualTo(fingerprint)
                .isNotEqualTo(FilterFingerprint.of(43L, "", SortOrder.CREATED_AT_DESC));
    }

    private static String flipLastPayloadByte(String token) {
        byte[] raw = Base64.getUrlDecoder().decode(token);
        raw[raw.length - 1] ^= 0x01;
        return Base64.getUrlEncoder().withoutPadding().encodeToString(raw);
    }
}
