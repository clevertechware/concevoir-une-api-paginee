package fr.clevertechware.pagination.web.dto;

import fr.clevertechware.pagination.domain.Transaction;

import java.time.format.DateTimeFormatter;

/**
 * @param id        a string, not a number: a {@code bigint} goes past JavaScript's
 *                  {@code Number.MAX_SAFE_INTEGER} and a client parsing it as JSON would lose bits
 *                  in silence
 * @param createdAt RFC 3339 UTC, {@code Z} suffix
 */
public record TransactionResponse(
        String id,
        long accountId,
        long amountCents,
        String label,
        String createdAt) {

    public static TransactionResponse from(Transaction transaction) {
        return new TransactionResponse(
                Long.toString(transaction.id()),
                transaction.accountId(),
                transaction.amountCents(),
                transaction.label(),
                DateTimeFormatter.ISO_INSTANT.format(transaction.createdAt().toInstant()));
    }
}
