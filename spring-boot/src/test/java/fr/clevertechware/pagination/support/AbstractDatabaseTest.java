package fr.clevertechware.pagination.support;

import org.junit.jupiter.api.Tag;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.testcontainers.service.connection.ServiceConnection;
import org.testcontainers.containers.PostgreSQLContainer;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.Objects;

/**
 * One PostgreSQL container for the whole suite, seeded once.
 *
 * <p>The schema comes from {@code ../sql/01-schema.sql}, the very file the shared compose stack
 * applies: an index that does not match the one in production would make every plan assertion here
 * meaningless. The application itself never applies a schema — it is a read-only client.
 */
@Tag("integration")
@SpringBootTest
public abstract class AbstractDatabaseTest {

    private static final Path SHARED_SCHEMA = Path.of("../sql/01-schema.sql");
    private static final String TEST_SEED = "/sql/test-seed.sql";

    @ServiceConnection
    protected static final PostgreSQLContainer<?> POSTGRES = new PostgreSQLContainer<>("postgres:18.4")
            .withDatabaseName("pagination")
            .withUsername("postgres")
            .withPassword("pagination");

    static {
        POSTGRES.start();
        executeScript(readSharedSchema());
        executeScript(readTestSeed());
    }

    protected static Connection openConnection() throws SQLException {
        return DriverManager.getConnection(
                POSTGRES.getJdbcUrl(), POSTGRES.getUsername(), POSTGRES.getPassword());
    }

    protected static void execute(String sql) {
        try (Connection connection = openConnection(); Statement statement = connection.createStatement()) {
            statement.execute(sql);
        } catch (SQLException e) {
            throw new IllegalStateException("failed to run SQL: " + sql, e);
        }
    }

    /** One statement at a time: a multi-statement string is an implicit transaction, and VACUUM refuses one. */
    private static void executeScript(String script) {
        String withoutComments = script.replaceAll("(?m)--.*$", "");
        for (String statement : withoutComments.split(";")) {
            if (!statement.isBlank()) {
                execute(statement);
            }
        }
    }

    private static String readSharedSchema() {
        try {
            return Files.readString(SHARED_SCHEMA);
        } catch (IOException e) {
            throw new IllegalStateException("the shared schema is expected at " + SHARED_SCHEMA.toAbsolutePath(), e);
        }
    }

    private static String readTestSeed() {
        try (var stream = Objects.requireNonNull(
                AbstractDatabaseTest.class.getResourceAsStream(TEST_SEED), TEST_SEED + " is missing")) {
            return new String(stream.readAllBytes(), StandardCharsets.UTF_8);
        } catch (IOException e) {
            throw new IllegalStateException("failed to read " + TEST_SEED, e);
        }
    }
}
