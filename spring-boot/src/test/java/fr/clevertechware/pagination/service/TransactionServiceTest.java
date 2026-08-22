package fr.clevertechware.pagination.service;

import fr.clevertechware.pagination.cursor.Cursor;
import fr.clevertechware.pagination.cursor.CursorCodec;
import fr.clevertechware.pagination.cursor.FilterFingerprint;
import fr.clevertechware.pagination.domain.CursorFilterMismatchException;
import fr.clevertechware.pagination.domain.KeysetPage;
import fr.clevertechware.pagination.domain.ListQuery;
import fr.clevertechware.pagination.domain.SortOrder;
import fr.clevertechware.pagination.domain.Transaction;
import fr.clevertechware.pagination.repository.TransactionRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;

import java.nio.charset.StandardCharsets;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.OffsetDateTime;
import java.time.ZoneOffset;
import java.util.List;
import java.util.stream.IntStream;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatExceptionOfType;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class TransactionServiceTest {

    private static final OffsetDateTime EPOCH = OffsetDateTime.parse("2021-01-08T01:46:40Z");
    private static final Clock CLOCK = Clock.fixed(Instant.parse("2026-01-01T00:00:00Z"), ZoneOffset.UTC);

    private final TransactionRepository repository = mock(TransactionRepository.class);
    private final CursorCodec codec = new CursorCodec(
            "test-key".getBytes(StandardCharsets.UTF_8), Duration.ofHours(72), CLOCK);

    private TransactionService service;

    @BeforeEach
    void setUp() {
        service = new TransactionService(repository, codec);
    }

    @Test
    void asksTheDatabaseForOneRowMoreThanThePageSoHasMoreCostsARowNotACount() {
        when(repository.findFirstPage(any(), anyInt())).thenReturn(rows(21));
        ArgumentCaptor<Integer> requestedLimit = ArgumentCaptor.forClass(Integer.class);

        service.list(unfiltered(20, null));

        verify(repository).findFirstPage(any(), requestedLimit.capture());
        assertThat(requestedLimit.getValue()).isEqualTo(21);
    }

    @Test
    void neverLetsTheExtraRowFetchedForHasMoreReachTheClient() {
        when(repository.findFirstPage(any(), anyInt())).thenReturn(rows(21));

        KeysetPage page = service.list(unfiltered(20, null));

        assertThat(page.rows()).hasSize(20);
        assertThat(page.hasMore()).isTrue();
        assertThat(page.nextCursor()).isNotNull();
    }

    @Test
    void endsTheWalkWithANullCursorWhenTheDatabaseReturnsNoExtraRow() {
        when(repository.findFirstPage(any(), anyInt())).thenReturn(rows(12));

        KeysetPage page = service.list(unfiltered(20, null));

        assertThat(page.rows()).hasSize(12);
        assertThat(page.hasMore()).isFalse();
        assertThat(page.nextCursor()).isNull();
    }

    @Test
    void pointsTheNextCursorAtTheLastRowActuallyHandedToTheClient() {
        when(repository.findFirstPage(any(), anyInt())).thenReturn(rows(21));

        Cursor next = codec.decode(service.list(unfiltered(20, null)).nextCursor());

        assertThat(next.id()).isEqualTo(20L);
        assertThat(next.createdAt().toInstant()).isEqualTo(EPOCH.plusSeconds(20 * 20L).toInstant());
    }

    @Test
    void routesTheFirstPageAndTheFollowingOnesToTwoDistinctStatements() {
        when(repository.findFirstPageForAccount(anyLong(), any(), anyInt())).thenReturn(rows(21));
        String cursor = service.list(filtered(42L, 20, null)).nextCursor();
        when(repository.findAfterCursorForAccount(anyLong(), any(), anyLong(), any(), anyInt()))
                .thenReturn(rows(5));

        service.list(filtered(42L, 20, cursor));

        verify(repository).findFirstPageForAccount(anyLong(), any(), anyInt());
        verify(repository).findAfterCursorForAccount(anyLong(), any(), anyLong(), any(), anyInt());
    }

    @Test
    void rejectsACursorReplayedOnAnotherAccountRatherThanServingAnInconsistentPage() {
        when(repository.findFirstPageForAccount(anyLong(), any(), anyInt())).thenReturn(rows(21));
        String cursorForAccount42 = service.list(filtered(42L, 20, null)).nextCursor();

        assertThatExceptionOfType(CursorFilterMismatchException.class)
                .isThrownBy(() -> service.list(filtered(43L, 20, cursorForAccount42)));
    }

    @Test
    void rejectsACursorReplayedWithTheSortReversed() {
        when(repository.findFirstPage(any(), anyInt())).thenReturn(rows(21));
        String descendingCursor = service.list(unfiltered(20, null)).nextCursor();

        ListQuery ascending = new ListQuery(
                ListQuery.NO_ACCOUNT_FILTER, "", SortOrder.CREATED_AT_ASC, 20, descendingCursor);

        assertThatExceptionOfType(CursorFilterMismatchException.class).isThrownBy(() -> service.list(ascending));
    }

    @Test
    void rejectsACursorReplayedWithAnotherStatusFilter() {
        when(repository.findFirstPage(any(), anyInt())).thenReturn(rows(21));
        String cursorWithoutStatus = service.list(unfiltered(20, null)).nextCursor();

        ListQuery withStatus = new ListQuery(
                ListQuery.NO_ACCOUNT_FILTER, "SETTLED", SortOrder.CREATED_AT_DESC, 20, cursorWithoutStatus);

        assertThatExceptionOfType(CursorFilterMismatchException.class).isThrownBy(() -> service.list(withStatus));
    }

    @Test
    void stampsTheNextCursorWithTheFingerprintOfTheCurrentFilters() {
        when(repository.findFirstPageForAccount(anyLong(), any(), anyInt())).thenReturn(rows(21));

        Cursor next = codec.decode(service.list(filtered(42L, 20, null)).nextCursor());

        assertThat(next.fingerprint())
                .isEqualTo(FilterFingerprint.of(42L, "", SortOrder.CREATED_AT_DESC));
    }

    @Test
    void countsTheRowsForRealOnlyWhenTheClientAsksForAnExactTotal() {
        when(repository.exactRowCount()).thenReturn(10_000_000L);

        assertThat(service.total(true)).isEqualTo(10_000_000L);
        verify(repository, never()).estimatedRowCount();
    }

    @Test
    void answersTheTotalWithThePlannerEstimateByDefault() {
        when(repository.estimatedRowCount()).thenReturn(9_998_400L);

        assertThat(service.total(false)).isEqualTo(9_998_400L);
        verify(repository, never()).exactRowCount();
    }

    private static ListQuery unfiltered(int limit, String cursor) {
        return new ListQuery(ListQuery.NO_ACCOUNT_FILTER, "", SortOrder.CREATED_AT_DESC, limit, cursor);
    }

    private static ListQuery filtered(long accountId, int limit, String cursor) {
        return new ListQuery(accountId, "", SortOrder.CREATED_AT_DESC, limit, cursor);
    }

    private static List<Transaction> rows(int count) {
        return IntStream.rangeClosed(1, count)
                .mapToObj(i -> new Transaction(i, 42L, 100L * i, "VIREMENT " + i, EPOCH.plusSeconds(20L * i)))
                .toList();
    }
}
