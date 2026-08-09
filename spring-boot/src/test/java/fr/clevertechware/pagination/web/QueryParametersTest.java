package fr.clevertechware.pagination.web;

import fr.clevertechware.pagination.domain.ExportQuery;
import fr.clevertechware.pagination.domain.ListQuery;
import fr.clevertechware.pagination.domain.SortOrder;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatExceptionOfType;

class QueryParametersTest {

    @Test
    void capsAnOversizedLimitInsteadOfRejectingIt() {
        assertThat(QueryParameters.limit("10000", 20, 100)).isEqualTo(100);
    }

    @Test
    void keepsALimitBelowTheCap() {
        assertThat(QueryParameters.limit("37", 20, 100)).isEqualTo(37);
    }

    @Test
    void fallsBackToTheDefaultLimitWhenTheParameterIsAbsent() {
        assertThat(QueryParameters.limit(null, 20, 100)).isEqualTo(20);
    }

    @ParameterizedTest
    @ValueSource(strings = {"-1", "abc", "20.5"})
    void rejectsANegativeOrNonNumericLimitAsInvalidLimit(String raw) {
        assertThatExceptionOfType(InvalidParameterException.class)
                .isThrownBy(() -> QueryParameters.limit(raw, 20, 100))
                .matches(e -> e.code().equals("invalid_limit"));
    }

    @Test
    void readsAnExplicitZeroLimitAsNoPreferenceRatherThanAsAnError() {
        assertThat(QueryParameters.limit("0", 20, 100)).isEqualTo(20);
        assertThat(QueryParameters.size("0", 20, 100)).isEqualTo(20);
    }

    @ParameterizedTest
    @ValueSource(strings = {"0", "-3", "one"})
    void rejectsAPageBelowOneOrNonNumericAsInvalidPage(String raw) {
        assertThatExceptionOfType(InvalidParameterException.class)
                .isThrownBy(() -> QueryParameters.page(raw))
                .matches(e -> e.code().equals("invalid_page"));
    }

    @Test
    void defaultsToTheFirstPageWhenThePageParameterIsAbsent() {
        assertThat(QueryParameters.page(null)).isEqualTo(1);
    }

    @ParameterizedTest
    @ValueSource(strings = {"created_at", "created_at:desc,id", "amount:desc", "CREATED_AT:DESC"})
    void rejectsAnySortOutsideTheTwoTheContractAllows(String raw) {
        assertThatExceptionOfType(InvalidParameterException.class)
                .isThrownBy(() -> QueryParameters.sort(raw))
                .matches(e -> e.code().equals("invalid_sort"));
    }

    @Test
    void defaultsToDescendingSort() {
        assertThat(QueryParameters.sort(null)).isEqualTo(SortOrder.CREATED_AT_DESC);
    }

    @Test
    void acceptsTheAscendingSortOfTheContract() {
        assertThat(QueryParameters.sort("created_at:asc")).isEqualTo(SortOrder.CREATED_AT_ASC);
    }

    @ParameterizedTest
    @ValueSource(strings = {"abc", "-1", "0", "4.2"})
    void rejectsANonPositiveOrNonNumericAccountIdAsInvalidAccountId(String raw) {
        assertThatExceptionOfType(InvalidParameterException.class)
                .isThrownBy(() -> QueryParameters.accountId(raw))
                .matches(e -> e.code().equals("invalid_account_id"));
    }

    @Test
    void treatsAnAbsentAccountIdAsTheSameZeroTheFingerprintUses() {
        assertThat(QueryParameters.accountId(null)).isEqualTo(ListQuery.NO_ACCOUNT_FILTER);
    }

    @Test
    void treatsAnAbsentAfterIdAsTheStartOfTheExportWalk() {
        assertThat(QueryParameters.afterId(null)).isEqualTo(ExportQuery.FROM_THE_BEGINNING);
    }

    @Test
    void treatsABlankCursorAsAFirstPageRequest() {
        assertThat(QueryParameters.cursor("  ")).isNull();
    }

    @Test
    void treatsAnAbsentStatusAsTheEmptyStringTheFingerprintUses() {
        assertThat(QueryParameters.status(null)).isEmpty();
    }
}
