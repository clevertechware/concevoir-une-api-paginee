package fr.clevertechware.pagination.domain;

import java.time.OffsetDateTime;

public record Transaction(
        long id,
        long accountId,
        long amountCents,
        String label,
        OffsetDateTime createdAt) {
}
