package fr.clevertechware.pagination.web.dto;

/**
 * The answer to "give me a total": planner statistics by default, and an {@code exact} flag that
 * says so. A real {@code COUNT(*)} on the article's dataset costs 101 ms, some 4 000 times the page
 * it would accompany, so a client only gets one by asking for {@code exact=true}.
 */
public record TotalResponse(long estimate, boolean exact) {
}
