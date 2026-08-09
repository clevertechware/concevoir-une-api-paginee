package fr.clevertechware.pagination.cursor;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;

/**
 * @param key        HMAC key. Committed here for the demo only; in production it comes from the
 *                   configuration store and rotating it invalidates every cursor in flight
 * @param timeToLive retention window past which a cursor answers 410 instead of a page
 */
@ConfigurationProperties(prefix = "pagination.cursor")
public record CursorProperties(String key, Duration timeToLive) {
}
