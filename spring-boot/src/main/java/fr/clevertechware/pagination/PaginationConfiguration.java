package fr.clevertechware.pagination;

import fr.clevertechware.pagination.cursor.CursorCodec;
import fr.clevertechware.pagination.cursor.CursorProperties;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import java.nio.charset.StandardCharsets;
import java.time.Clock;

@Configuration
@EnableConfigurationProperties(CursorProperties.class)
public class PaginationConfiguration {

    @Bean
    public Clock clock() {
        return Clock.systemUTC();
    }

    @Bean
    public CursorCodec cursorCodec(CursorProperties properties, Clock clock) {
        return new CursorCodec(
                properties.key().getBytes(StandardCharsets.UTF_8), properties.timeToLive(), clock);
    }
}
