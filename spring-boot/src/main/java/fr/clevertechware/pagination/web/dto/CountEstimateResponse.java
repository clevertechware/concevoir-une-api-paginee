package fr.clevertechware.pagination.web.dto;

/**
 * The answer to "give me a total": planner statistics, and an {@code exact} flag that says so.
 * A real {@code COUNT(*)} on the article's dataset costs 101 ms, some 4 000 times the page it
 * would accompany.
 */
public record CountEstimateResponse(long estimate, boolean exact) {
}
