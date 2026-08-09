package fr.clevertechware.pagination.web;

import fr.clevertechware.pagination.cursor.CursorExpiredException;
import fr.clevertechware.pagination.cursor.InvalidCursorException;
import fr.clevertechware.pagination.domain.CursorFilterMismatchException;
import fr.clevertechware.pagination.web.dto.ErrorResponse;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;

@RestControllerAdvice
public class ApiExceptionHandler {

    private static final Logger log = LoggerFactory.getLogger(ApiExceptionHandler.class);

    @ExceptionHandler(InvalidParameterException.class)
    public ResponseEntity<ErrorResponse> onInvalidParameter(InvalidParameterException e) {
        return ResponseEntity.badRequest().body(ErrorResponse.of(e.code(), e.getMessage()));
    }

    @ExceptionHandler(InvalidCursorException.class)
    public ResponseEntity<ErrorResponse> onInvalidCursor(InvalidCursorException e) {
        return ResponseEntity.badRequest().body(ErrorResponse.of("invalid_cursor", e.getMessage()));
    }

    @ExceptionHandler(CursorFilterMismatchException.class)
    public ResponseEntity<ErrorResponse> onCursorFilterMismatch(CursorFilterMismatchException e) {
        return ResponseEntity.badRequest().body(ErrorResponse.of("cursor_filter_mismatch", e.getMessage()));
    }

    /**
     * 410 rather than 400: the request was well-formed, it is the position that no longer exists,
     * which is exactly what the client needs to know to decide to restart the walk.
     */
    @ExceptionHandler(CursorExpiredException.class)
    public ResponseEntity<ErrorResponse> onCursorExpired(CursorExpiredException e) {
        return ResponseEntity.status(HttpStatus.GONE).body(ErrorResponse.of("cursor_expired", e.getMessage()));
    }

    /** The internal message is logged, never serialised. */
    @ExceptionHandler(Exception.class)
    public ResponseEntity<ErrorResponse> onUnexpectedFailure(Exception e) {
        log.error("unhandled failure while serving the request", e);
        return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                .body(ErrorResponse.of("internal_error", "unexpected error"));
    }
}
