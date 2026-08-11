package fr.clevertechware.pagination.web;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import fr.clevertechware.pagination.cursor.Cursor;
import fr.clevertechware.pagination.cursor.CursorCodec;
import fr.clevertechware.pagination.cursor.CursorProperties;
import fr.clevertechware.pagination.cursor.FilterFingerprint;
import fr.clevertechware.pagination.domain.SortOrder;
import fr.clevertechware.pagination.support.AbstractDatabaseTest;
import org.hamcrest.Matchers;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.MvcResult;

import java.nio.charset.StandardCharsets;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.OffsetDateTime;
import java.time.ZoneOffset;
import java.util.Base64;

import static org.assertj.core.api.Assertions.assertThat;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

/** The HTTP contract of spec/contract.md, served by the real application against a real PostgreSQL. */
@AutoConfigureMockMvc
class TransactionApiIT extends AbstractDatabaseTest {

    private static final ObjectMapper JSON = new ObjectMapper();

    @Autowired
    private MockMvc mockMvc;

    @Autowired
    private CursorProperties cursorProperties;

    @Test
    void servesTheFirstPageWithTheEnvelopeOfTheContract() throws Exception {
        mockMvc.perform(get("/v1/transactions").param("account_id", "42").param("limit", "3"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.data.length()").value(3))
                .andExpect(jsonPath("$.page.has_more").value(true))
                .andExpect(jsonPath("$.page.next").isNotEmpty())
                .andExpect(jsonPath("$.data[0].account_id").value(42))
                .andExpect(jsonPath("$.data[0].amount_cents").isNumber())
                .andExpect(jsonPath("$.data[0].label").isString());
    }

    @Test
    void serialisesTheBigintIdAsAStringSoAJavaScriptClientCannotLoseBits() throws Exception {
        mockMvc.perform(get("/v1/transactions").param("limit", "1"))
                .andExpect(jsonPath("$.data[0].id").isString());
    }

    @Test
    void serialisesCreatedAtAsRfc3339Utc() throws Exception {
        mockMvc.perform(get("/v1/transactions").param("limit", "1"))
                .andExpect(jsonPath("$.data[0].created_at").value(Matchers.endsWith("Z")));
    }

    @Test
    void neverReturnsATotal() throws Exception {
        mockMvc.perform(get("/v1/transactions").param("limit", "1"))
                .andExpect(jsonPath("$.page.total").doesNotExist())
                .andExpect(jsonPath("$.total").doesNotExist());
    }

    @Test
    void endsTheWalkWithANullNextCursor() throws Exception {
        mockMvc.perform(get("/v1/transactions").param("account_id", "99").param("limit", "100"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.data.length()").value(50))
                .andExpect(jsonPath("$.page.has_more").value(false))
                .andExpect(jsonPath("$.page.next").value(Matchers.nullValue()));
    }

    @Test
    void capsAnOversizedLimitAtOneHundredInsteadOfRejectingTheRequest() throws Exception {
        mockMvc.perform(get("/v1/transactions").param("limit", "10000"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.data.length()").value(100));
    }

    @Test
    void continuesTheWalkFromTheCursorItJustHandedOut() throws Exception {
        JsonNode firstPage = body(mockMvc.perform(
                get("/v1/transactions").param("account_id", "42").param("limit", "5")).andReturn());
        String lastIdOfFirstPage = firstPage.get("data").get(4).get("id").textValue();

        JsonNode secondPage = body(mockMvc.perform(get("/v1/transactions")
                        .param("account_id", "42")
                        .param("limit", "5")
                        .param("cursor", firstPage.get("page").get("next").textValue()))
                .andExpect(status().isOk())
                .andReturn());

        assertThat(secondPage.get("data").get(0).get("id").textValue()).isNotEqualTo(lastIdOfFirstPage);
        assertThat(Long.parseLong(secondPage.get("data").get(0).get("id").textValue()))
                .isLessThan(Long.parseLong(lastIdOfFirstPage));
    }

    @Test
    void rejectsATamperedCursorWithFourHundredInvalidCursor() throws Exception {
        String tampered = flipLastByte(cursorFor("42"));

        mockMvc.perform(get("/v1/transactions").param("account_id", "42").param("cursor", tampered))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.error.code").value("invalid_cursor"))
                .andExpect(jsonPath("$.error.message").isString());
    }

    @Test
    void rejectsACursorReplayedOnAnotherAccountWithFourHundredCursorFilterMismatch() throws Exception {
        String cursorForAccount42 = cursorFor("42");

        mockMvc.perform(get("/v1/transactions").param("account_id", "7").param("cursor", cursorForAccount42))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.error.code").value("cursor_filter_mismatch"));
    }

    @Test
    void rejectsACursorOlderThanTheRetentionWindowWithFourHundredAndTenCursorExpired() throws Exception {
        mockMvc.perform(get("/v1/transactions").param("account_id", "42").param("cursor", staleCursor()))
                .andExpect(status().isGone())
                .andExpect(jsonPath("$.error.code").value("cursor_expired"));
    }

    @Test
    void rejectsANonNumericLimitWithFourHundredInvalidLimit() throws Exception {
        mockMvc.perform(get("/v1/transactions").param("limit", "twenty"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.error.code").value("invalid_limit"));
    }

    @Test
    void rejectsAnUnknownSortWithFourHundredInvalidSort() throws Exception {
        mockMvc.perform(get("/v1/transactions").param("sort", "amount:desc"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.error.code").value("invalid_sort"));
    }

    @Test
    void rejectsANonNumericAccountIdWithFourHundredInvalidAccountId() throws Exception {
        mockMvc.perform(get("/v1/transactions").param("account_id", "moi"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.error.code").value("invalid_account_id"));
    }

    @Test
    void rejectsAPageBelowOneWithFourHundredInvalidPage() throws Exception {
        mockMvc.perform(get("/v1/transactions/offset").param("page", "0"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.error.code").value("invalid_page"));
    }

    @Test
    void servesTheOffsetCounterExampleWithItsOwnPageEnvelope() throws Exception {
        mockMvc.perform(get("/v1/transactions/offset")
                        .param("account_id", "42")
                        .param("page", "2")
                        .param("size", "5"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.data.length()").value(5))
                .andExpect(jsonPath("$.page.page").value(2))
                .andExpect(jsonPath("$.page.size").value(5))
                .andExpect(jsonPath("$.page.has_more").value(true));
    }

    @Test
    void answersTheTotalAsAnEstimateAndSaysSo() throws Exception {
        mockMvc.perform(get("/v1/transactions/count-estimate"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.exact").value(false))
                .andExpect(jsonPath("$.estimate").isNumber());
    }

    @Test
    void reportsTheDatabaseAsReachableOnHealthz() throws Exception {
        mockMvc.perform(get("/healthz"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.status").value("ok"));
    }

    private String cursorFor(String accountId) throws Exception {
        JsonNode page = body(mockMvc.perform(
                get("/v1/transactions").param("account_id", accountId).param("limit", "3")).andReturn());
        return page.get("page").get("next").textValue();
    }

    private String staleCursor() {
        Instant issuedFourDaysAgo = Instant.now().minus(Duration.ofHours(96));
        CursorCodec pastCodec = new CursorCodec(
                cursorProperties.key().getBytes(StandardCharsets.UTF_8),
                cursorProperties.timeToLive(),
                Clock.fixed(issuedFourDaysAgo, ZoneOffset.UTC));

        Cursor stale = pastCodec.issue(
                OffsetDateTime.parse("2015-01-02T00:00:00Z"),
                1000L,
                true,
                FilterFingerprint.of(42L, "", SortOrder.CREATED_AT_DESC));

        return pastCodec.encode(stale);
    }

    private static String flipLastByte(String token) {
        byte[] raw = Base64.getUrlDecoder().decode(token);
        raw[raw.length - 1] ^= 0x01;
        return Base64.getUrlEncoder().withoutPadding().encodeToString(raw);
    }

    private static JsonNode body(MvcResult result) throws Exception {
        return JSON.readTree(result.getResponse().getContentAsString(StandardCharsets.UTF_8));
    }
}
