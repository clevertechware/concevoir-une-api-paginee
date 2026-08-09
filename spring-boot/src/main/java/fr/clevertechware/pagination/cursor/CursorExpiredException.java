package fr.clevertechware.pagination.cursor;

/**
 * The token was well-formed and properly signed, but older than the retention window.
 * The request was fine; the position no longer exists. That is a 410, not a 400.
 */
public class CursorExpiredException extends RuntimeException {

    public CursorExpiredException(String message) {
        super(message);
    }
}
