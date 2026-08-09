package fr.clevertechware.pagination.domain;

import java.util.List;

public record OffsetPage(List<Transaction> rows, int page, int size, boolean hasMore) {
}
