package fr.clevertechware.pagination.domain;

import java.util.List;

public record ExportPage(List<Transaction> rows, Long nextAfterId, boolean hasMore) {
}
