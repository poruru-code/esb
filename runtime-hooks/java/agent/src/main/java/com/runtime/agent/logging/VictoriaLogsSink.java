// Where: runtime-hooks/java/agent/src/main/java/com/runtime/agent/logging/VictoriaLogsSink.java
// What: VictoriaLogs sender and log line parser.
// Why: Forward stdout/CloudWatch logs without relying on app code.
package com.runtime.agent.logging;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.runtime.agent.util.TraceContextAccessor;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.LinkedHashMap;
import java.util.Locale;
import java.util.Map;

public final class VictoriaLogsSink {
    private static final ObjectMapper MAPPER = new ObjectMapper().findAndRegisterModules();
    private static final VictoriaLogsSink INSTANCE = new VictoriaLogsSink();

    private final String baseUrl;

    private VictoriaLogsSink() {
        String raw = System.getenv("VICTORIALOGS_URL");
        if (raw == null || raw.trim().isEmpty()) {
            this.baseUrl = null;
        } else {
            this.baseUrl = raw.replaceAll("/+$", "");
        }
    }

    public static boolean enabled() {
        return INSTANCE.baseUrl != null && !INSTANCE.baseUrl.isEmpty();
    }

    public static void sendLine(String line) {
        INSTANCE.sendLineInternal(line);
    }

    public static void send(Map<String, Object> entry) {
        INSTANCE.sendInternal(entry);
    }

    private void sendLineInternal(String line) {
        if (!enabled() || line == null) {
            return;
        }
        Map<String, Object> entry = parseLine(line);
        sendInternal(entry);
    }

    private Map<String, Object> parseLine(String line) {
        Map<String, Object> entry = null;
        try {
            entry = MAPPER.readValue(line, Map.class);
        } catch (Exception ignored) {
            // fallback to plain message
        }

        if (entry == null || entry.isEmpty()) {
            entry = new LinkedHashMap<>();
            entry.put("_time", now());
            entry.put("level", detectLevel(line));
            entry.put("message", line);
        }

        if (!entry.containsKey("_time")) {
            if (entry.containsKey("timestamp")) {
                entry.put("_time", entry.get("timestamp"));
            } else {
                entry.put("_time", now());
            }
        }
        if (!entry.containsKey("message")) {
            entry.put("message", line);
        }
        if (!entry.containsKey("level")) {
            entry.put("level", detectLevel(line));
        }

        attachMetadata(entry);
        return entry;
    }

    private void attachMetadata(Map<String, Object> entry) {
        String functionName = System.getenv("AWS_LAMBDA_FUNCTION_NAME");
        if (functionName == null || functionName.isEmpty()) {
            functionName = "lambda-unknown";
        }
        entry.putIfAbsent("container_name", functionName);
        entry.putIfAbsent("job", "lambda");

        String traceId = TraceContextAccessor.traceId();
        String existingTrace = entry.get("trace_id") == null ? null : entry.get("trace_id").toString();
        if (traceId != null && !traceId.isEmpty()) {
            if (existingTrace == null
                    || existingTrace.isBlank()
                    || "not-found".equalsIgnoreCase(existingTrace)) {
                entry.put("trace_id", traceId);
            } else {
                entry.putIfAbsent("trace_id", traceId);
            }
        }
        String requestId = TraceContextAccessor.requestId();
        if (requestId != null && !requestId.isEmpty()) {
            entry.putIfAbsent("aws_request_id", requestId);
        }
    }

    private void sendInternal(Map<String, Object> entry) {
        if (!enabled()) {
            return;
        }
        try {
            String containerName = String.valueOf(entry.getOrDefault("container_name", "lambda-unknown"));
            String params = "_stream_fields=container_name,job&_msg_field=message&_time_field=_time" +
                    "&container_name=" + URLEncoder.encode(containerName, StandardCharsets.UTF_8) +
                    "&job=" + URLEncoder.encode("lambda", StandardCharsets.UTF_8);
            URL url = new URL(baseUrl + "/insert/jsonline?" + params);

            byte[] payload = MAPPER.writeValueAsBytes(entry);
            HttpURLConnection conn = (HttpURLConnection) url.openConnection();
            conn.setConnectTimeout(500);
            conn.setReadTimeout(1000);
            conn.setRequestMethod("POST");
            conn.setDoOutput(true);
            conn.setRequestProperty("Content-Type", "application/json");
            conn.getOutputStream().write(payload);
            conn.getOutputStream().flush();
            conn.getInputStream().close();
        } catch (Exception ignored) {
            // best effort
        }
    }

    private String detectLevel(String message) {
        if (message == null) {
            return "INFO";
        }
        String upper = message.toUpperCase(Locale.ROOT);
        if (upper.contains("ERROR") || upper.contains("CRIT")) {
            return "ERROR";
        }
        if (upper.contains("WARN")) {
            return "WARNING";
        }
        if (upper.contains("DEBUG") || upper.contains("TRACE")) {
            return "DEBUG";
        }
        return "INFO";
    }

    private String now() {
        return Instant.now().truncatedTo(ChronoUnit.MILLIS).toString();
    }
}
