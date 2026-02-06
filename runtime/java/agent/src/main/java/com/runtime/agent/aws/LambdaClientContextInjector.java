// Where: runtime/java/agent/src/main/java/com/runtime/agent/aws/LambdaClientContextInjector.java
// What: Injects trace_id into Lambda Invoke client context payloads.
// Why: Preserve trace propagation in RIE/runtime without app changes.
package com.runtime.agent.aws;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.runtime.agent.util.ReflectionUtils;
import com.runtime.agent.util.TraceContextAccessor;
import java.nio.charset.StandardCharsets;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.Map;

public final class LambdaClientContextInjector {
    private static final ObjectMapper MAPPER = new ObjectMapper().findAndRegisterModules();
    private static final TypeReference<Map<String, Object>> MAP_TYPE = new TypeReference<>() {};

    private LambdaClientContextInjector() {}

    public static Object inject(Object request) {
        if (request == null) {
            return null;
        }
        String traceId = TraceContextAccessor.traceId();
        if (traceId == null || traceId.isEmpty()) {
            traceId = System.getenv("_X_AMZN_TRACE_ID");
        }
        if (traceId == null || traceId.isEmpty()) {
            return null;
        }

        Object existingContext = ReflectionUtils.invoke(request, "clientContext");
        String encoded = existingContext instanceof String ? (String) existingContext : null;

        Map<String, Object> ctx = decodeClientContext(encoded);
        Map<String, Object> custom = ensureCustom(ctx);
        Object existingTrace = custom.get("trace_id");
        if (existingTrace == null || existingTrace.toString().isBlank()) {
            custom.put("trace_id", traceId);
        }

        String updated = encodeClientContext(ctx);
        if (updated == null) {
            return null;
        }

        Object builder = ReflectionUtils.invoke(request, "toBuilder");
        if (builder == null) {
            return null;
        }
        ReflectionUtils.invoke(builder, "clientContext", updated);
        Object built = ReflectionUtils.invoke(builder, "build");
        return built != null ? built : null;
    }

    private static Map<String, Object> decodeClientContext(String encoded) {
        if (encoded == null || encoded.isEmpty()) {
            return new LinkedHashMap<>();
        }
        try {
            byte[] raw = Base64.getDecoder().decode(encoded);
            String json = new String(raw, StandardCharsets.UTF_8);
            Map<String, Object> parsed = MAPPER.readValue(json, MAP_TYPE);
            return parsed != null ? parsed : new LinkedHashMap<>();
        } catch (Exception ignored) {
            return new LinkedHashMap<>();
        }
    }

    private static String encodeClientContext(Map<String, Object> ctx) {
        try {
            String json = MAPPER.writeValueAsString(ctx);
            return Base64.getEncoder().encodeToString(json.getBytes(StandardCharsets.UTF_8));
        } catch (Exception ignored) {
            return null;
        }
    }

    @SuppressWarnings("unchecked")
    private static Map<String, Object> ensureCustom(Map<String, Object> ctx) {
        Object custom = ctx.get("custom");
        if (custom instanceof Map<?, ?> customMap) {
            return (Map<String, Object>) customMap;
        }
        Map<String, Object> created = new LinkedHashMap<>();
        ctx.put("custom", created);
        return created;
    }
}
