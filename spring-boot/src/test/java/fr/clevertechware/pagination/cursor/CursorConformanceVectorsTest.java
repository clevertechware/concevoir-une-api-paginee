package fr.clevertechware.pagination.cursor;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import fr.clevertechware.pagination.domain.SortOrder;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.MethodSource;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.LocalDateTime;
import java.time.OffsetDateTime;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Base64;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * The cross-language conformance suite: {@code spec/cursor-vectors.json} is shared with the Go
 * implementation, so a token this codec produces for a given position is the token the other one
 * produces, and either API accepts the other's cursors.
 */
class CursorConformanceVectorsTest {

    private static final Path VECTORS = Path.of("../spec/cursor-vectors.json");
    private static final int SIGNATURE_LENGTH = 32;
    private static final DateTimeFormatter PAYLOAD_TIMESTAMP =
            DateTimeFormatter.ofPattern("uuuu-MM-dd'T'HH:mm:ss.SSSSSS'Z'");

    private record Vector(
            String name,
            long accountId,
            String status,
            SortOrder sort,
            String fingerprint,
            String payload,
            String token,
            byte[] hmacKey) {

        @Override
        public String toString() {
            return name;
        }
    }

    @ParameterizedTest
    @MethodSource("vectors")
    void recomputesTheFingerprintOfTheSharedVectorFromItsFilters(Vector vector) {
        assertThat(FilterFingerprint.of(vector.accountId(), vector.status(), vector.sort()))
                .isEqualTo(vector.fingerprint());
    }

    @ParameterizedTest
    @MethodSource("vectors")
    void producesTheCanonicalPayloadOfTheSharedVectorByteForByte(Vector vector) {
        String token = codecFor(vector).encode(cursorOf(vector));

        byte[] raw = Base64.getUrlDecoder().decode(token);
        String payload = new String(
                Arrays.copyOfRange(raw, SIGNATURE_LENGTH, raw.length), StandardCharsets.UTF_8);

        assertThat(payload).isEqualTo(vector.payload());
    }

    @ParameterizedTest
    @MethodSource("vectors")
    void producesTheSignedTokenOfTheSharedVectorByteForByte(Vector vector) {
        assertThat(codecFor(vector).encode(cursorOf(vector))).isEqualTo(vector.token());
    }

    @ParameterizedTest
    @MethodSource("vectors")
    void decodesTheSharedVectorTokenBackIntoItsOriginalPosition(Vector vector) {
        Cursor decoded = codecFor(vector).decode(vector.token());

        assertThat(decoded).isEqualTo(cursorOf(vector));
    }

    private static CursorCodec codecFor(Vector vector) {
        Cursor cursor = cursorOf(vector);
        Clock justAfterIssuance = Clock.fixed(
                Instant.ofEpochSecond(cursor.issuedAt()).plusSeconds(3600), ZoneOffset.UTC);
        return new CursorCodec(vector.hmacKey(), Duration.ofHours(72), justAfterIssuance);
    }

    private static Cursor cursorOf(Vector vector) {
        JsonNode payload = readTree(vector.payload());
        return new Cursor(
                payload.get("v").intValue(),
                OffsetDateTime.of(LocalDateTime.parse(payload.get("c").textValue(), PAYLOAD_TIMESTAMP), ZoneOffset.UTC),
                payload.get("i").longValue(),
                payload.get("d").booleanValue(),
                payload.get("f").textValue(),
                payload.get("t").longValue());
    }

    private static List<Vector> vectors() throws IOException {
        JsonNode root = new ObjectMapper().readTree(Files.readString(VECTORS));
        byte[] key = root.get("hmac_key_utf8").textValue().getBytes(StandardCharsets.UTF_8);

        List<Vector> vectors = new ArrayList<>();
        for (JsonNode node : root.get("vectors")) {
            JsonNode filters = node.get("filters");
            vectors.add(new Vector(
                    node.get("name").textValue(),
                    filters.get("account_id").longValue(),
                    filters.get("status").textValue(),
                    SortOrder.fromValue(filters.get("sort").textValue()),
                    node.get("fingerprint").textValue(),
                    node.get("payload").textValue(),
                    node.get("token").textValue(),
                    key));
        }
        return vectors;
    }

    private static JsonNode readTree(String json) {
        try {
            return new ObjectMapper().readTree(json);
        } catch (IOException e) {
            throw new IllegalStateException(e);
        }
    }
}
