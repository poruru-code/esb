// Where: cli/internal/infra/build/assets/java/src/com/runtime/lambda/HandlerWrapper.java
// What: Java Lambda handler wrapper for logging/trace hooks.
// Why: Provide brand-neutral runtime hooks and handler delegation.
package com.runtime.lambda;

import com.amazonaws.services.lambda.runtime.ClientContext;
import com.amazonaws.services.lambda.runtime.Context;
import com.amazonaws.services.lambda.runtime.RequestStreamHandler;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.io.PrintStream;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.lang.reflect.Modifier;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;

public final class HandlerWrapper implements RequestStreamHandler {
    private static final String ENV_ORIGINAL_HANDLER = "LAMBDA_ORIGINAL_HANDLER";
    private static final String ENV_VICTORIALOGS_URL = "VICTORIALOGS_URL";
    private static final String ENV_FUNCTION_NAME = "AWS_LAMBDA_FUNCTION_NAME";
    private static final String DEFAULT_FUNCTION_NAME = "lambda-unknown";
    private static final String DEFAULT_JOB = "lambda";

    private static final ObjectMapper MAPPER = new ObjectMapper().findAndRegisterModules();
    private static final ThreadLocal<String> TRACE_ID = new ThreadLocal<>();
    private static final ThreadLocal<String> REQUEST_ID = new ThreadLocal<>();

    @Override
    public void handleRequest(InputStream input, OutputStream output, Context context) throws IOException {
        String handler = getenvTrim(ENV_ORIGINAL_HANDLER);
        if (handler == null || handler.isEmpty()) {
            throw new IllegalStateException("LAMBDA_ORIGINAL_HANDLER is required");
        }

        byte[] payload = readPayload(input);
        TRACE_ID.set(resolveTraceId(context, payload));
        REQUEST_ID.set(context == null ? null : context.getAwsRequestId());

        PrintStream originalOut = System.out;
        PrintStream originalErr = System.err;

        VictoriaLogsClient logClient = new VictoriaLogsClient(getenvTrim(ENV_VICTORIALOGS_URL));
        PrintStream hookedOut = originalOut;
        PrintStream hookedErr = originalErr;
        if (logClient.isEnabled()) {
            hookedOut = new PrintStream(new TeeOutputStream(originalOut, logClient), true, StandardCharsets.UTF_8);
            hookedErr = new PrintStream(new TeeOutputStream(originalErr, logClient), true, StandardCharsets.UTF_8);
            System.setOut(hookedOut);
            System.setErr(hookedErr);
        }

        try {
            invokeOriginal(handler, payload, output, context);
        } catch (Exception e) {
            logClient.sendError("handler invocation failed", e);
            throw rethrow(e);
        } finally {
            System.setOut(originalOut);
            System.setErr(originalErr);
            try {
                hookedOut.flush();
                hookedErr.flush();
            } catch (Exception ignored) {
                // best effort
            }
            TRACE_ID.remove();
            REQUEST_ID.remove();
        }
    }

    private static RuntimeException rethrow(Exception e) throws IOException {
        if (e instanceof InvocationTargetException && e.getCause() instanceof Exception) {
            e = (Exception) e.getCause();
        }
        if (e instanceof RuntimeException) {
            throw (RuntimeException) e;
        }
        if (e instanceof IOException) {
            throw (IOException) e;
        }
        return new RuntimeException(e);
    }

    private static void invokeOriginal(String handler, byte[] payload, OutputStream output, Context context) throws Exception {
        HandlerTarget target = HandlerTarget.parse(handler);
        Class<?> clazz = Class.forName(target.className, true, Thread.currentThread().getContextClassLoader());
        ResolvedMethod resolved = resolveMethod(clazz, target.methodName);
        Object instance = Modifier.isStatic(resolved.method.getModifiers()) ? null : clazz.getDeclaredConstructor().newInstance();

        if (resolved.mode == MethodMode.STREAM) {
            invokeStream(resolved.method, instance, payload, output, context);
            return;
        }

        Object payloadObject = resolveInput(payload, resolved.inputType);
        Object result;
        if (resolved.hasContext) {
            result = resolved.method.invoke(instance, payloadObject, context);
        } else if (resolved.mode == MethodMode.NO_ARG) {
            result = resolved.method.invoke(instance);
        } else {
            result = resolved.method.invoke(instance, payloadObject);
        }

        if (!resolved.returnsVoid && output != null) {
            writeOutput(output, result);
        }
    }

    private static void invokeStream(Method method, Object instance, byte[] payload, OutputStream output, Context context) throws Exception {
        InputStream input = new ByteArrayInputStream(payload == null ? new byte[0] : payload);
        Class<?>[] params = method.getParameterTypes();
        if (params.length == 3) {
            method.invoke(instance, input, output, context);
            return;
        }
        if (params.length == 2) {
            method.invoke(instance, input, output);
            return;
        }
        throw new IllegalStateException("unsupported stream handler signature: " + method);
    }

    private static Object resolveInput(byte[] payload, Class<?> inputType) throws IOException {
        if (inputType == null) {
            return null;
        }
        if (payload == null || payload.length == 0) {
            return null;
        }
        if (inputType == byte[].class) {
            return payload;
        }
        if (inputType == String.class) {
            return new String(payload, StandardCharsets.UTF_8);
        }
        if (inputType == InputStream.class) {
            return new ByteArrayInputStream(payload);
        }
        return MAPPER.readValue(payload, inputType);
    }

    private static void writeOutput(OutputStream output, Object result) throws IOException {
        if (result == null) {
            return;
        }
        if (result instanceof byte[]) {
            output.write((byte[]) result);
            return;
        }
        if (result instanceof String) {
            output.write(((String) result).getBytes(StandardCharsets.UTF_8));
            return;
        }
        MAPPER.writeValue(output, result);
    }

    private static ResolvedMethod resolveMethod(Class<?> clazz, String methodName) throws NoSuchMethodException {
        String name = methodName == null || methodName.isEmpty() ? "handleRequest" : methodName;

        Method method = findMethod(clazz, name, InputStream.class, OutputStream.class, Context.class);
        if (method != null) {
            return new ResolvedMethod(method, MethodMode.STREAM, null, false);
        }
        method = findMethod(clazz, name, InputStream.class, OutputStream.class);
        if (method != null) {
            return new ResolvedMethod(method, MethodMode.STREAM, null, false);
        }

        Method objectMethod = null;
        Method singleMethod = null;
        Method noArgMethod = null;

        for (Method candidate : clazz.getMethods()) {
            if (!candidate.getName().equals(name)) {
                continue;
            }
            Class<?>[] params = candidate.getParameterTypes();
            if (params.length == 2 && Context.class.isAssignableFrom(params[1])) {
                objectMethod = candidate;
                break;
            }
            if (params.length == 1 && singleMethod == null) {
                singleMethod = candidate;
            }
            if (params.length == 0 && noArgMethod == null) {
                noArgMethod = candidate;
            }
        }

        if (objectMethod != null) {
            Class<?> inputType = objectMethod.getParameterTypes()[0];
            return new ResolvedMethod(objectMethod, MethodMode.OBJECT, inputType, true);
        }
        if (singleMethod != null) {
            Class<?> inputType = singleMethod.getParameterTypes()[0];
            return new ResolvedMethod(singleMethod, MethodMode.OBJECT, inputType, false);
        }
        if (noArgMethod != null) {
            return new ResolvedMethod(noArgMethod, MethodMode.NO_ARG, null, false);
        }

        throw new NoSuchMethodException("handler method not found: " + name);
    }

    private static Method findMethod(Class<?> clazz, String name, Class<?>... params) {
        try {
            return clazz.getMethod(name, params);
        } catch (NoSuchMethodException e) {
            return null;
        }
    }

    private static byte[] readPayload(InputStream input) throws IOException {
        if (input == null) {
            return new byte[0];
        }
        return input.readAllBytes();
    }

    private static String resolveTraceId(Context context, byte[] payload) {
        String traceId = resolveTraceIdFromContext(context);
        if (traceId != null) {
            return traceId;
        }
        traceId = normalizeTraceId(getenvTrim("_X_AMZN_TRACE_ID"));
        if (traceId != null) {
            return traceId;
        }
        return extractTraceIdFromPayload(payload);
    }

    private static String resolveTraceIdFromContext(Context context) {
        if (context == null) {
            return null;
        }
        ClientContext clientContext = context.getClientContext();
        if (clientContext != null) {
            Map<String, String> custom = clientContext.getCustom();
            if (custom != null) {
                String trace = custom.get("trace_id");
                if (trace == null || trace.isEmpty()) {
                    trace = custom.get("_X_AMZN_TRACE_ID");
                }
                if (trace != null && !trace.isEmpty()) {
                    return normalizeTraceId(trace);
                }
            }
        }
        return null;
    }

    private static String extractTraceIdFromPayload(byte[] payload) {
        if (payload == null || payload.length == 0) {
            return null;
        }
        try {
            Map<String, Object> event = MAPPER.readValue(payload, Map.class);
            String trace = extractTraceHeader(event.get("headers"));
            if (trace == null) {
                trace = extractTraceHeader(event.get("multiValueHeaders"));
            }
            if (trace == null) {
                Object requestContext = event.get("requestContext");
                if (requestContext instanceof Map) {
                    Object traceValue = ((Map<?, ?>) requestContext).get("traceId");
                    if (traceValue != null) {
                        trace = traceValue.toString();
                    }
                }
            }
            return normalizeTraceId(trace);
        } catch (Exception ignored) {
            return null;
        }
    }

    private static String extractTraceHeader(Object headersObj) {
        if (!(headersObj instanceof Map)) {
            return null;
        }
        Map<?, ?> headers = (Map<?, ?>) headersObj;
        for (Map.Entry<?, ?> entry : headers.entrySet()) {
            if (entry.getKey() == null) {
                continue;
            }
            String key = entry.getKey().toString();
            if (!"x-amzn-trace-id".equalsIgnoreCase(key)) {
                continue;
            }
            Object value = entry.getValue();
            if (value == null) {
                return null;
            }
            if (value instanceof List) {
                List<?> list = (List<?>) value;
                if (!list.isEmpty() && list.get(0) != null) {
                    return list.get(0).toString();
                }
                return null;
            }
            return value.toString();
        }
        return null;
    }

    private static String normalizeTraceId(String trace) {
        if (trace == null) {
            return null;
        }
        String trimmed = trace.trim();
        if (trimmed.isEmpty()) {
            return null;
        }
        int rootIndex = trimmed.indexOf("Root=");
        if (rootIndex >= 0) {
            int start = rootIndex + 5;
            int end = trimmed.indexOf(';', start);
            if (end == -1) {
                end = trimmed.length();
            }
            if (start < end) {
                return trimmed.substring(start, end);
            }
        }
        return trimmed;
    }

    private static String currentTraceId() {
        return TRACE_ID.get();
    }

    private static String currentRequestId() {
        return REQUEST_ID.get();
    }

    private static String getenvTrim(String key) {
        String value = System.getenv(key);
        if (value == null) {
            return null;
        }
        value = value.trim();
        return value.isEmpty() ? null : value;
    }

    private enum MethodMode {
        STREAM,
        OBJECT,
        NO_ARG
    }

    private static final class ResolvedMethod {
        private final Method method;
        private final MethodMode mode;
        private final Class<?> inputType;
        private final boolean hasContext;
        private final boolean returnsVoid;

        private ResolvedMethod(Method method, MethodMode mode, Class<?> inputType, boolean hasContext) {
            this.method = method;
            this.mode = mode;
            this.inputType = inputType;
            this.hasContext = hasContext;
            this.returnsVoid = method.getReturnType() == Void.TYPE;
        }
    }

    private static final class HandlerTarget {
        private final String className;
        private final String methodName;

        private HandlerTarget(String className, String methodName) {
            this.className = className;
            this.methodName = methodName;
        }

        private static HandlerTarget parse(String handler) {
            String trimmed = handler == null ? "" : handler.trim();
            int sep = trimmed.indexOf("::");
            if (sep == -1) {
                return new HandlerTarget(trimmed, null);
            }
            String className = trimmed.substring(0, sep).trim();
            String methodName = trimmed.substring(sep + 2).trim();
            return new HandlerTarget(className, methodName);
        }
    }

    private static final class TeeOutputStream extends OutputStream {
        private final OutputStream original;
        private final VictoriaLogsClient client;
        private final ByteArrayOutputStream buffer = new ByteArrayOutputStream();

        private TeeOutputStream(OutputStream original, VictoriaLogsClient client) {
            this.original = original;
            this.client = client;
        }

        @Override
        public void write(int b) throws IOException {
            original.write(b);
            if (b == '\n') {
                flushBuffer();
            } else {
                buffer.write(b);
            }
        }

        @Override
        public void write(byte[] b, int off, int len) throws IOException {
            original.write(b, off, len);
            for (int i = off; i < off + len; i++) {
                byte current = b[i];
                if (current == '\n') {
                    flushBuffer();
                } else {
                    buffer.write(current);
                }
            }
        }

        @Override
        public void flush() throws IOException {
            original.flush();
            flushBuffer();
        }

        private void flushBuffer() {
            if (!client.isEnabled()) {
                buffer.reset();
                return;
            }
            if (buffer.size() == 0) {
                return;
            }
            String line = buffer.toString(StandardCharsets.UTF_8);
            buffer.reset();
            if (!line.isBlank()) {
                client.sendLine(line);
            }
        }
    }

    private static final class VictoriaLogsClient {
        private final String baseUrl;

        private VictoriaLogsClient(String baseUrl) {
            this.baseUrl = baseUrl == null ? null : baseUrl.replaceAll("/+$", "");
        }

        private boolean isEnabled() {
            return baseUrl != null && !baseUrl.isEmpty();
        }

        private void sendLine(String line) {
            Map<String, Object> entry = parseLogEntry(line);
            send(entry);
        }

        private void sendError(String message, Exception e) {
            Map<String, Object> entry = new LinkedHashMap<>();
            entry.put("_time", now());
            entry.put("level", "ERROR");
            entry.put("message", message);
            entry.put("exception", e.toString());
            attachMetadata(entry);
            send(entry);
        }

        private Map<String, Object> parseLogEntry(String line) {
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
            String functionName = getenvTrim(ENV_FUNCTION_NAME);
            entry.putIfAbsent("container_name", functionName == null ? DEFAULT_FUNCTION_NAME : functionName);
            entry.putIfAbsent("job", DEFAULT_JOB);
            String traceId = currentTraceId();
            Object existingTrace = entry.get("trace_id");
            String normalizedExisting = existingTrace == null ? null : normalizeTraceId(existingTrace.toString());
            if (traceId != null && !traceId.isEmpty()) {
                if (normalizedExisting == null
                        || normalizedExisting.isEmpty()
                        || "not-found".equalsIgnoreCase(normalizedExisting)) {
                    entry.put("trace_id", traceId);
                } else if (!normalizedExisting.equals(existingTrace.toString())) {
                    entry.put("trace_id", normalizedExisting);
                }
            } else if (normalizedExisting != null && existingTrace != null
                    && !normalizedExisting.equals(existingTrace.toString())) {
                entry.put("trace_id", normalizedExisting);
            }
            String requestId = currentRequestId();
            if (requestId != null && !requestId.isEmpty()) {
                entry.putIfAbsent("aws_request_id", requestId);
            }
        }

        private void send(Map<String, Object> entry) {
            if (!isEnabled()) {
                return;
            }
            try {
                String containerName = String.valueOf(entry.getOrDefault("container_name", DEFAULT_FUNCTION_NAME));
                String params = "_stream_fields=container_name,job&_msg_field=message&_time_field=_time" +
                        "&container_name=" + URLEncoder.encode(containerName, StandardCharsets.UTF_8) +
                        "&job=" + URLEncoder.encode(DEFAULT_JOB, StandardCharsets.UTF_8);
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
}
