package fr.clevertechware.pagination.web.dto;

/** The answer to "give me a total": planner statistics by default, and an {@code exact} flag that says so. */
public record TotalResponse(long estimate, boolean exact) {
}
