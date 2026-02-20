// Where: runtime-hooks/java/agent/src/main/java/com/runtime/agent/aws/CloudWatchLogsRequestGuard.java
// What: Guard logic for CloudWatch Logs request argument shape.
// Why: Avoid skipping SDK calls for Consumer overloads and unsupported request shapes.
package com.runtime.agent.aws;

import java.lang.reflect.Method;
import java.time.Instant;
import java.util.Locale;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.function.Consumer;

public final class CloudWatchLogsRequestGuard {
    private static final String REQUEST_PACKAGE_PREFIX =
            "software.amazon.awssdk.services.cloudwatchlogs.model.";
    private static final String DIAGNOSTIC_ENV = "ESB_JAVA_CLOUDWATCH_DIAGNOSTIC";
    private static final Set<String> SUPPORTED_CONSUMER_OVERLOADS = Set.of(
            "createLogGroup",
            "createLogStream",
            "deleteLogGroup",
            "deleteLogStream",
            "describeLogGroups",
            "describeLogStreams");
    private static final Set<String> CLOUDWATCH_METHODS = Set.of(
            "putLogEvents",
            "createLogGroup",
            "createLogStream",
            "deleteLogGroup",
            "deleteLogStream",
            "describeLogGroups",
            "describeLogStreams");
    private static final Set<String> WARNED_FALLBACKS = ConcurrentHashMap.newKeySet();
    private static final boolean DIAGNOSTIC_ENABLED = isDiagnosticEnabled();

    private CloudWatchLogsRequestGuard() {}

    public static boolean isSupported(Method method, Object request) {
        if (method == null || request == null) {
            emitDiagnostic("DEBUG", method, request, "method_or_request_null", false);
            return false;
        }

        String methodName = method.getName();
        Class<?>[] parameterTypes = method.getParameterTypes();
        if (parameterTypes.length == 0) {
            warnOnFallback(method, request, "no_arguments");
            emitDiagnostic("DEBUG", method, request, "no_arguments", false);
            return false;
        }

        if (Consumer.class.isAssignableFrom(parameterTypes[0])) {
            boolean supported = SUPPORTED_CONSUMER_OVERLOADS.contains(methodName);
            if (!supported) {
                warnOnFallback(method, request, "unsupported_consumer_overload");
            }
            emitDiagnostic(
                    "DEBUG",
                    method,
                    request,
                    supported ? "supported_consumer_overload" : "unsupported_consumer_overload",
                    supported);
            return supported;
        }

        String requestClassName = request.getClass().getName();
        boolean supported =
                requestClassName.startsWith(REQUEST_PACKAGE_PREFIX) && requestClassName.endsWith("Request");
        if (!supported) {
            warnOnFallback(method, request, "unsupported_request_type");
        }
        emitDiagnostic(
                "DEBUG",
                method,
                request,
                supported ? "supported_request_object" : "unsupported_request_type",
                supported);
        return supported;
    }

    private static void warnOnFallback(Method method, Object request, String reason) {
        if (method == null || !CLOUDWATCH_METHODS.contains(method.getName())) {
            return;
        }
        String firstParameterType = firstParameterType(method);
        String warnKey = method.getName() + "|" + firstParameterType + "|" + reason;
        if (!WARNED_FALLBACKS.add(warnKey)) {
            return;
        }
        emitLog(
                "WARNING",
                "CloudWatch call bypassed local mock and will use original SDK path",
                method,
                request,
                reason,
                false);
    }

    private static void emitDiagnostic(
            String level,
            Method method,
            Object request,
            String reason,
            boolean supported
    ) {
        if (!DIAGNOSTIC_ENABLED) {
            return;
        }
        emitLog(
                level,
                "CloudWatch guard decision",
                method,
                request,
                reason,
                supported);
    }

    private static void emitLog(
            String level,
            String message,
            Method method,
            Object request,
            String reason,
            boolean supported
    ) {
        StringBuilder sb = new StringBuilder(512);
        sb.append("{");
        appendField(sb, "_time", Instant.now().toString(), true);
        appendField(sb, "level", level, true);
        appendField(sb, "logger", "javaagent.cloudwatch.guard", true);
        appendField(sb, "message", message, true);
        appendField(sb, "method", method == null ? "null" : method.getName(), true);
        appendField(sb, "first_param", firstParameterType(method), true);
        appendField(sb, "request_type", requestType(request), true);
        appendField(sb, "reason", reason, true);
        appendField(sb, "diagnostic_env", DIAGNOSTIC_ENV, true);
        appendField(sb, "hint", "set ESB_JAVA_CLOUDWATCH_DIAGNOSTIC=true for per-call decisions", true);
        appendBooleanField(sb, "supported", supported);
        sb.append("}");
        System.out.println(sb);
    }

    private static void appendField(StringBuilder sb, String key, String value, boolean withTrailingComma) {
        sb.append("\"").append(escapeJson(key)).append("\":");
        sb.append("\"").append(escapeJson(value)).append("\"");
        if (withTrailingComma) {
            sb.append(",");
        }
    }

    private static void appendBooleanField(StringBuilder sb, String key, boolean value) {
        sb.append("\"").append(escapeJson(key)).append("\":").append(value);
    }

    private static String firstParameterType(Method method) {
        if (method == null) {
            return "none";
        }
        Class<?>[] parameterTypes = method.getParameterTypes();
        if (parameterTypes.length == 0 || parameterTypes[0] == null) {
            return "none";
        }
        return parameterTypes[0].getName();
    }

    private static String requestType(Object request) {
        return request == null ? "null" : request.getClass().getName();
    }

    private static String escapeJson(String value) {
        if (value == null) {
            return "";
        }
        StringBuilder escaped = new StringBuilder(value.length() + 16);
        for (int i = 0; i < value.length(); i++) {
            char c = value.charAt(i);
            switch (c) {
                case '"' -> escaped.append("\\\"");
                case '\\' -> escaped.append("\\\\");
                case '\n' -> escaped.append("\\n");
                case '\r' -> escaped.append("\\r");
                case '\t' -> escaped.append("\\t");
                default -> escaped.append(c);
            }
        }
        return escaped.toString();
    }

    private static boolean isDiagnosticEnabled() {
        String value = System.getenv(DIAGNOSTIC_ENV);
        if (value == null) {
            return false;
        }
        String normalized = value.trim().toLowerCase(Locale.ROOT);
        return normalized.equals("1")
                || normalized.equals("true")
                || normalized.equals("yes")
                || normalized.equals("on");
    }
}
