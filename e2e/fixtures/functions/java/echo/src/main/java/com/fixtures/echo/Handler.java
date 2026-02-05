// Where: e2e/fixtures/functions/java/echo/src/main/java/com/fixtures/echo/Handler.java
// What: Java echo Lambda fixture for E2E coverage.
// Why: Provide a minimal Java runtime target mirroring the Python echo behavior.
package com.fixtures.echo;

import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.Base64;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;

public final class Handler {
    private static final String DEFAULT_USER = "anonymous";
    private static final String DEFAULT_TRACE_ID = "not-found";
    private static final String DEFAULT_FUNCTION = "lambda-echo-java";

    public Map<String, Object> handleRequest(Map<String, Object> event) {
        Map<String, Object> safeEvent = event == null ? Collections.emptyMap() : event;

        if (Boolean.TRUE.equals(safeEvent.get("ping"))) {
            return createResponse(200, "pong");
        }

        String rawMessage = resolveMessage(safeEvent);
        String username = resolveUsername(safeEvent);
        String message = "Echo: " + (rawMessage == null || rawMessage.isBlank() ? "Hello" : rawMessage);
        String traceId = getenvOrDefault("_X_AMZN_TRACE_ID", DEFAULT_TRACE_ID);

        emitLog("INFO", message, traceId);
        emitLog("DEBUG", "Debug log for quality test", traceId);

        String responseBody = buildResponseBody(message, username);
        return createResponse(200, responseBody);
    }

    private static String resolveMessage(Map<String, Object> event) {
        Object body = event.get("body");
        if (body instanceof Map<?, ?> bodyMap) {
            Object message = bodyMap.get("message");
            return message == null ? null : message.toString();
        }
        if (body instanceof String bodyText) {
            String payload = bodyText;
            if (Boolean.TRUE.equals(event.get("isBase64Encoded"))) {
                payload = decodeBase64(bodyText);
            }
            String extracted = findJsonStringField(payload, "message");
            if (extracted != null) {
                return extracted;
            }
        }
        Object directMessage = event.get("message");
        return directMessage == null ? null : directMessage.toString();
    }

    private static String resolveUsername(Map<String, Object> event) {
        Object requestContext = event.get("requestContext");
        if (requestContext instanceof Map<?, ?> ctxMap) {
            Object authorizer = ctxMap.get("authorizer");
            if (authorizer instanceof Map<?, ?> authMap) {
                Object username = authMap.get("cognito:username");
                if (username != null) {
                    String value = username.toString();
                    if (!value.isBlank()) {
                        return value;
                    }
                }
            }
        }
        return DEFAULT_USER;
    }

    private static Map<String, Object> createResponse(int statusCode, String body) {
        Map<String, Object> response = new LinkedHashMap<>();
        response.put("statusCode", statusCode);
        Map<String, String> headers = new LinkedHashMap<>();
        headers.put("Content-Type", "application/json");
        response.put("headers", headers);
        response.put("body", body);
        return response;
    }

    private static String buildResponseBody(String message, String user) {
        StringBuilder builder = new StringBuilder();
        builder.append("{");
        builder.append("\"success\":true,");
        builder.append("\"message\":\"").append(escapeJson(message)).append("\",");
        builder.append("\"user\":\"").append(escapeJson(user)).append("\"");
        builder.append("}");
        return builder.toString();
    }

    private static void emitLog(String level, String message, String traceId) {
        String timestamp = Instant.now().truncatedTo(ChronoUnit.MILLIS).toString();
        String functionName = getenvOrDefault("AWS_LAMBDA_FUNCTION_NAME", DEFAULT_FUNCTION);

        StringBuilder builder = new StringBuilder();
        builder.append("{");
        builder.append("\"_time\":\"").append(escapeJson(timestamp)).append("\",");
        builder.append("\"level\":\"").append(escapeJson(level)).append("\",");
        builder.append("\"trace_id\":\"").append(escapeJson(traceId)).append("\",");
        builder.append("\"message\":\"").append(escapeJson(message)).append("\",");
        builder.append("\"function\":\"").append(escapeJson(functionName)).append("\"");
        builder.append("}");
        System.out.println(builder.toString());
    }

    private static String decodeBase64(String value) {
        try {
            byte[] decoded = Base64.getDecoder().decode(value);
            return new String(decoded, StandardCharsets.UTF_8);
        } catch (IllegalArgumentException ignored) {
            return value;
        }
    }

    private static String findJsonStringField(String json, String key) {
        if (json == null) {
            return null;
        }
        String needle = "\"" + key + "\"";
        int idx = json.indexOf(needle);
        if (idx == -1) {
            return null;
        }
        idx = json.indexOf(':', idx + needle.length());
        if (idx == -1) {
            return null;
        }
        idx++;
        while (idx < json.length() && Character.isWhitespace(json.charAt(idx))) {
            idx++;
        }
        if (idx >= json.length() || json.charAt(idx) != '"') {
            return null;
        }
        idx++;
        StringBuilder value = new StringBuilder();
        boolean escape = false;
        while (idx < json.length()) {
            char current = json.charAt(idx++);
            if (escape) {
                switch (current) {
                    case '"':
                    case '\\':
                    case '/':
                        value.append(current);
                        break;
                    case 'b':
                        value.append('\b');
                        break;
                    case 'f':
                        value.append('\f');
                        break;
                    case 'n':
                        value.append('\n');
                        break;
                    case 'r':
                        value.append('\r');
                        break;
                    case 't':
                        value.append('\t');
                        break;
                    case 'u':
                        if (idx + 3 < json.length()) {
                            String hex = json.substring(idx, idx + 4);
                            try {
                                int code = Integer.parseInt(hex, 16);
                                value.append((char) code);
                                idx += 4;
                            } catch (NumberFormatException ignored) {
                                value.append('u').append(hex);
                                idx += 4;
                            }
                        }
                        break;
                    default:
                        value.append(current);
                        break;
                }
                escape = false;
                continue;
            }
            if (current == '\\') {
                escape = true;
                continue;
            }
            if (current == '"') {
                return value.toString();
            }
            value.append(current);
        }
        return null;
    }

    private static String escapeJson(String value) {
        if (value == null) {
            return "";
        }
        StringBuilder escaped = new StringBuilder();
        for (int i = 0; i < value.length(); i++) {
            char current = value.charAt(i);
            switch (current) {
                case '"':
                    escaped.append("\\\"");
                    break;
                case '\\':
                    escaped.append("\\\\");
                    break;
                case '\b':
                    escaped.append("\\b");
                    break;
                case '\f':
                    escaped.append("\\f");
                    break;
                case '\n':
                    escaped.append("\\n");
                    break;
                case '\r':
                    escaped.append("\\r");
                    break;
                case '\t':
                    escaped.append("\\t");
                    break;
                default:
                    if (current < 0x20) {
                        escaped.append(String.format("\\u%04x", (int) current));
                    } else {
                        escaped.append(current);
                    }
                    break;
            }
        }
        return escaped.toString();
    }

    private static String getenvOrDefault(String key, String defaultValue) {
        String value = System.getenv(key);
        if (value == null || value.isBlank()) {
            return defaultValue;
        }
        return value;
    }
}
