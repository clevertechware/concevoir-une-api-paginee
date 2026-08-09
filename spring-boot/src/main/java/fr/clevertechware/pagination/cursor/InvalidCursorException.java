package fr.clevertechware.pagination.cursor;

/** Unreadable token, broken signature, or a format version this build does not know. */
public class InvalidCursorException extends RuntimeException {

    public InvalidCursorException(String message) {
        super(message);
    }
}
