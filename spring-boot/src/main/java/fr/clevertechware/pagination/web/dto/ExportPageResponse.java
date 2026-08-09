package fr.clevertechware.pagination.web.dto;

import fr.clevertechware.pagination.domain.ExportPage;

import java.util.List;

public record ExportPageResponse(List<TransactionResponse> data, PageInfo page) {

    public record PageInfo(String nextAfterId, boolean hasMore) {
    }

    public static ExportPageResponse from(ExportPage page) {
        String nextAfterId = page.nextAfterId() == null ? null : Long.toString(page.nextAfterId());
        return new ExportPageResponse(
                page.rows().stream().map(TransactionResponse::from).toList(),
                new PageInfo(nextAfterId, page.hasMore()));
    }
}
