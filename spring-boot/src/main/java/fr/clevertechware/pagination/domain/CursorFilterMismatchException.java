package fr.clevertechware.pagination.domain;

/**
 * The cursor was issued for other filters than the ones on the current request. Answering with a
 * page here would mean handing the client rows from a walk it never started.
 */
public class CursorFilterMismatchException extends RuntimeException {

    public CursorFilterMismatchException(String message) {
        super(message);
    }
}
