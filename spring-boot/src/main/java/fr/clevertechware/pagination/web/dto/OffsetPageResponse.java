package fr.clevertechware.pagination.web.dto;

import fr.clevertechware.pagination.domain.OffsetPage;

import java.util.List;

public record OffsetPageResponse(List<TransactionResponse> data, PageInfo page) {

    public record PageInfo(int page, int size, boolean hasMore) {
    }

    public static OffsetPageResponse from(OffsetPage page) {
        return new OffsetPageResponse(
                page.rows().stream().map(TransactionResponse::from).toList(),
                new PageInfo(page.page(), page.size(), page.hasMore()));
    }
}
