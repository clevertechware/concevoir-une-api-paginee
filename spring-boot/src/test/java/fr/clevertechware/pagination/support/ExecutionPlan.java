package fr.clevertechware.pagination.support;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/** Reads {@code EXPLAIN (ANALYZE, BUFFERS)} output so a test can assert on the plan itself. */
public final class ExecutionPlan {

    private static final String EXPLAIN = "EXPLAIN (ANALYZE, BUFFERS, COSTS OFF, TIMING OFF) ";
    private static final Pattern BLOCKS = Pattern.compile("\\b(?:hit|read)=(\\d+)");

    private ExecutionPlan() {
    }

    public static String of(Connection connection, String sql, Object... parameters) throws SQLException {
        try (PreparedStatement statement = connection.prepareStatement(EXPLAIN + sql)) {
            for (int i = 0; i < parameters.length; i++) {
                statement.setObject(i + 1, parameters[i]);
            }
            try (ResultSet rows = statement.executeQuery()) {
                StringBuilder plan = new StringBuilder();
                while (rows.next()) {
                    plan.append(rows.getString(1)).append('\n');
                }
                return plan.toString();
            }
        }
    }

    public static String ofStatement(Connection connection, String sql) throws SQLException {
        try (Statement statement = connection.createStatement();
             ResultSet rows = statement.executeQuery(EXPLAIN + sql)) {
            StringBuilder plan = new StringBuilder();
            while (rows.next()) {
                plan.append(rows.getString(1)).append('\n');
            }
            return plan.toString();
        }
    }

    /** Blocks the executor touched, cached or not — the cost figure the article compares across depths. */
    public static int blocksTouched(String plan) {
        Matcher matcher = BLOCKS.matcher(plan);
        int total = 0;
        while (matcher.find()) {
            total += Integer.parseInt(matcher.group(1));
        }
        return total;
    }

    /** Turns the JDBC placeholders of a production statement into the {@code $n} a PREPARE needs. */
    public static String withNumberedPlaceholders(String sql) {
        StringBuilder prepared = new StringBuilder(sql.length() + 16);
        int placeholder = 0;
        for (char c : sql.toCharArray()) {
            if (c == '?') {
                prepared.append('$').append(++placeholder);
            } else {
                prepared.append(c);
            }
        }
        return prepared.toString();
    }
}
