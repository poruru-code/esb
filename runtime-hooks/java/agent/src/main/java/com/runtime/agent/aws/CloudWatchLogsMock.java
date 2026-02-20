// Where: runtime-hooks/java/agent/src/main/java/com/runtime/agent/aws/CloudWatchLogsMock.java
// What: Local handler for CloudWatch Logs SDK calls.
// Why: Replace PutLogEvents with stdout/VictoriaLogs and avoid AWS calls.
package com.runtime.agent.aws;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.runtime.agent.logging.VictoriaLogsHook;
import com.runtime.agent.logging.VictoriaLogsSink;
import com.runtime.agent.util.ReflectionUtils;
import com.runtime.agent.util.TraceContextAccessor;
import java.lang.reflect.Method;
import java.lang.reflect.ParameterizedType;
import java.lang.reflect.Type;
import java.time.Instant;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.concurrent.CompletableFuture;

public final class CloudWatchLogsMock {
    private static final ObjectMapper MAPPER = new ObjectMapper().findAndRegisterModules();
    private static final DateTimeFormatter ISO_FMT = DateTimeFormatter.ISO_INSTANT.withZone(ZoneOffset.UTC);

    private CloudWatchLogsMock() {}

    public static Object handle(Method method, Object request) {
        if (method == null) {
            return null;
        }
        String name = method.getName();
        Object response = switch (name) {
            case "putLogEvents" -> {
                handlePutLogEvents(request);
                yield null;
            }
            case "createLogGroup",
                    "createLogStream",
                    "deleteLogGroup",
                    "deleteLogStream",
                    "describeLogGroups",
                    "describeLogStreams" -> null;
            default -> null;
        };

        if (response == null) {
            response = buildResponseFromMethod(method, name);
        }

        if (response == null) {
            return null;
        }
        if (CompletableFuture.class.isAssignableFrom(method.getReturnType())) {
            if (response instanceof CompletableFuture<?>) {
                return response;
            }
            return CompletableFuture.completedFuture(response);
        }
        return response;
    }

    private static void handlePutLogEvents(Object request) {
        if (request == null) {
            return;
        }
        String logGroup = asString(ReflectionUtils.invoke(request, "logGroupName"));
        String logStream = asString(ReflectionUtils.invoke(request, "logStreamName"));
        Object eventsObj = ReflectionUtils.invoke(request, "logEvents");
        List<?> events = eventsObj instanceof List<?> list ? list : Collections.emptyList();

        String containerName = System.getenv("AWS_LAMBDA_FUNCTION_NAME");
        if (containerName == null || containerName.isEmpty()) {
            containerName = "lambda-unknown";
        }

        for (Object event : events) {
            String message = asString(ReflectionUtils.invoke(event, "message"));
            Long timestamp = asLong(ReflectionUtils.invoke(event, "timestamp"));
            emitLogEntry(logGroup, logStream, containerName, message, timestamp);
        }
    }

    private static void emitLogEntry(
            String logGroup,
            String logStream,
            String containerName,
            String message,
            Long timestamp
    ) {
        long ts = timestamp == null ? System.currentTimeMillis() : timestamp;
        String level = detectLevel(message);
        String cleanMessage = stripLevelPrefix(message, level);

        Map<String, Object> entry = new LinkedHashMap<>();
        entry.put("_time", ISO_FMT.format(Instant.ofEpochMilli(ts)));
        entry.put("level", level);
        entry.put("message", cleanMessage);
        entry.put("log_group", logGroup == null ? "unknown" : logGroup);
        entry.put("log_stream", logStream == null ? "unknown" : logStream);
        entry.put("logger", "cloudwatch.logs.java");
        entry.put("container_name", containerName);
        entry.put("job", "lambda");

        String traceId = TraceContextAccessor.traceId();
        if (traceId != null && !traceId.isEmpty()) {
            entry.put("trace_id", traceId);
        }
        String requestId = TraceContextAccessor.requestId();
        if (requestId != null && !requestId.isEmpty()) {
            entry.put("aws_request_id", requestId);
        }

        try {
            String json = MAPPER.writeValueAsString(entry);
            System.out.println(json);
        } catch (Exception ignored) {
            System.out.println(cleanMessage);
        }

        if (!VictoriaLogsHook.isInstalled()) {
            VictoriaLogsSink.send(entry);
        }
    }

    private static Object buildResponseFromMethod(Method method, String methodName) {
        Class<?> returnType = method.getReturnType();
        if (returnType == null || returnType == Void.TYPE) {
            return null;
        }
        if (CompletableFuture.class.isAssignableFrom(returnType)) {
            return CompletableFuture.completedFuture(buildAsyncResponsePayload(method, methodName));
        }

        try {
            Object builder = ReflectionUtils.invokeStatic(returnType, "builder");
            if (builder == null) {
                return null;
            }

            if ("putLogEvents".equals(methodName)) {
                ReflectionUtils.invoke(builder, "nextSequenceToken", "mock-token");
            } else if ("describeLogGroups".equals(methodName)) {
                ReflectionUtils.invoke(builder, "logGroups", new ArrayList<>());
            } else if ("describeLogStreams".equals(methodName)) {
                ReflectionUtils.invoke(builder, "logStreams", new ArrayList<>());
            }

            return ReflectionUtils.invoke(builder, "build");
        } catch (Exception ignored) {
            return null;
        }
    }

    private static Object buildAsyncResponsePayload(Method method, String methodName) {
        if (method == null) {
            return null;
        }
        Type generic = method.getGenericReturnType();
        if (!(generic instanceof ParameterizedType parameterizedType)) {
            return null;
        }
        Type[] args = parameterizedType.getActualTypeArguments();
        if (args.length == 0) {
            return null;
        }
        Type responseType = args[0];
        if (!(responseType instanceof Class<?> responseClass)) {
            return null;
        }
        try {
            Object builder = ReflectionUtils.invokeStatic(responseClass, "builder");
            if (builder == null) {
                return null;
            }
            if ("putLogEvents".equals(methodName)) {
                ReflectionUtils.invoke(builder, "nextSequenceToken", "mock-token");
            } else if ("describeLogGroups".equals(methodName)) {
                ReflectionUtils.invoke(builder, "logGroups", new ArrayList<>());
            } else if ("describeLogStreams".equals(methodName)) {
                ReflectionUtils.invoke(builder, "logStreams", new ArrayList<>());
            }
            return ReflectionUtils.invoke(builder, "build");
        } catch (Exception ignored) {
            return null;
        }
    }

    private static String detectLevel(String message) {
        if (message == null) {
            return "INFO";
        }
        String upper = message.toUpperCase(Locale.ROOT);
        if (upper.startsWith("[DEBUG]") || upper.contains(" TRACE")) {
            return "DEBUG";
        }
        if (upper.startsWith("[WARN]") || upper.contains("WARN")) {
            return "WARNING";
        }
        if (upper.startsWith("[ERROR]") || upper.contains("ERROR") || upper.contains("CRIT")) {
            return "ERROR";
        }
        return "INFO";
    }

    private static String stripLevelPrefix(String message, String level) {
        if (message == null) {
            return "";
        }
        String prefix = "[" + level + "]";
        if (message.startsWith(prefix)) {
            return message.substring(prefix.length()).trim();
        }
        return message;
    }

    private static String asString(Object value) {
        return value == null ? null : value.toString();
    }

    private static Long asLong(Object value) {
        if (value instanceof Number num) {
            return num.longValue();
        }
        try {
            return value == null ? null : Long.parseLong(value.toString());
        } catch (Exception ignored) {
            return null;
        }
    }
}
