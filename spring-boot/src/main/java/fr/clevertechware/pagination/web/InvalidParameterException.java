package fr.clevertechware.pagination.web;

/** A query parameter the contract rejects, carrying the error code the contract names for it. */
public class InvalidParameterException extends RuntimeException {

    private final String code;

    public InvalidParameterException(String code, String message) {
        super(message);
        this.code = code;
    }

    public String code() {
        return code;
    }
}
