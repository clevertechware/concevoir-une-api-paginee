package fr.clevertechware.pagination.domain;

/**
 * @param afterId exclusive lower bound, {@link #FROM_THE_BEGINNING} to start the walk
 * @param limit   page size already capped by the web layer
 */
public record ExportQuery(long afterId, int limit) {

    public static final long FROM_THE_BEGINNING = 0L;
}
