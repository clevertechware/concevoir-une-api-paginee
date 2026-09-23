package fr.clevertechware.pagination.web.dto;

import fr.clevertechware.pagination.domain.KeysetPage;

import java.util.List;

public record KeysetPageResponse(List<TransactionResponse> data, PageInfo page) {

    /**
     * @param next     {@code null} at the end of the walk
     * @param hasMore  redundant with {@code next} on purpose: the contract then reads without its
     *                 documentation
     */
    public record PageInfo(String next, boolean hasMore) {
    }

    public static KeysetPageResponse from(KeysetPage page) {
        return new KeysetPageResponse(
                page.rows().stream().map(TransactionResponse::from).toList(),
                new PageInfo(page.nextCursor(), page.hasMore()));
    }
}
