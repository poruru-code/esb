// Where: runtime/java/agent/src/main/java/com/runtime/agent/aws/CloudWatchLogsMock.java
// What: Local handler for CloudWatch Logs SDK calls.
// Why: Replace PutLogEvents with stdout/VictoriaLogs and avoid AWS calls.
package com.runtime.agent.aws;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.runtime.agent.logging.VictoriaLogsHook;
import com.runtime.agent.logging.VictoriaLogsSink;
import com.runtime.agent.util.ReflectionUtils;
import com.runtime.agent.util.TraceContextAccessor;
import java.lang.reflect.Method;
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
        Object context = request != null ? request : method.getDeclaringClass();
        Object response = switch (name) {
            case "putLogEvents" -> handlePutLogEvents(request, context);
            case "createLogGroup" -> buildEmptyResponse(
                    "software.amazon.awssdk.services.cloudwatchlogs.model.CreateLogGroupResponse",
                    context);
            case "createLogStream" -> buildEmptyResponse(
                    "software.amazon.awssdk.services.cloudwatchlogs.model.CreateLogStreamResponse",
                    context);
            case "deleteLogGroup" -> buildEmptyResponse(
                    "software.amazon.awssdk.services.cloudwatchlogs.model.DeleteLogGroupResponse",
                    context);
            case "deleteLogStream" -> buildEmptyResponse(
                    "software.amazon.awssdk.services.cloudwatchlogs.model.DeleteLogStreamResponse",
                    context);
            case "describeLogGroups" -> buildDescribeLogGroupsResponse(context);
            case "describeLogStreams" -> buildDescribeLogStreamsResponse(context);
            default -> null;
        };

        if (response == null) {
            return null;
        }
        if (CompletableFuture.class.isAssignableFrom(method.getReturnType())) {
            return CompletableFuture.completedFuture(response);
        }
        return response;
    }

    private static Object handlePutLogEvents(Object request, Object context) {
        if (request == null) {
            return null;
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

        return buildPutLogEventsResponse(context);
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
        entry.put("logger", "aws.logs");
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

    private static Object buildPutLogEventsResponse(Object context) {
        try {
            Class<?> responseClass = ReflectionUtils.loadClass(
                    "software.amazon.awssdk.services.cloudwatchlogs.model.PutLogEventsResponse",
                    context);
            if (responseClass == null) {
                return null;
            }
            Object builder = ReflectionUtils.invokeStatic(responseClass, "builder");
            if (builder == null) {
                return null;
            }
            ReflectionUtils.invoke(builder, "nextSequenceToken", "mock-token");
            return ReflectionUtils.invoke(builder, "build");
        } catch (Exception ignored) {
            return null;
        }
    }

    private static Object buildDescribeLogGroupsResponse(Object context) {
        try {
            Class<?> responseClass = ReflectionUtils.loadClass(
                    "software.amazon.awssdk.services.cloudwatchlogs.model.DescribeLogGroupsResponse",
                    context);
            if (responseClass == null) {
                return null;
            }
            Object builder = ReflectionUtils.invokeStatic(responseClass, "builder");
            if (builder == null) {
                return null;
            }
            ReflectionUtils.invoke(builder, "logGroups", new ArrayList<>());
            return ReflectionUtils.invoke(builder, "build");
        } catch (Exception ignored) {
            return null;
        }
    }

    private static Object buildDescribeLogStreamsResponse(Object context) {
        try {
            Class<?> responseClass = ReflectionUtils.loadClass(
                    "software.amazon.awssdk.services.cloudwatchlogs.model.DescribeLogStreamsResponse",
                    context);
            if (responseClass == null) {
                return null;
            }
            Object builder = ReflectionUtils.invokeStatic(responseClass, "builder");
            if (builder == null) {
                return null;
            }
            ReflectionUtils.invoke(builder, "logStreams", new ArrayList<>());
            return ReflectionUtils.invoke(builder, "build");
        } catch (Exception ignored) {
            return null;
        }
    }

    private static Object buildEmptyResponse(String className, Object context) {
        try {
            Class<?> responseClass = ReflectionUtils.loadClass(className, context);
            if (responseClass == null) {
                return null;
            }
            Object builder = ReflectionUtils.invokeStatic(responseClass, "builder");
            if (builder == null) {
                return null;
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
