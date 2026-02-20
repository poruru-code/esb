// Where: runtime-hooks/java/agent/src/main/java/com/runtime/agent/util/TraceContextAccessor.java
// What: Reads trace/request IDs from the wrapper's TraceContext when available.
// Why: Enrich logs and propagation without hard dependency on wrapper classes.
package com.runtime.agent.util;

import java.lang.reflect.Method;

public final class TraceContextAccessor {
    private static final String TRACE_CONTEXT_CLASS = "com.runtime.lambda.TraceContext";
    private static volatile boolean loaded = false;
    private static volatile Method traceMethod;
    private static volatile Method requestMethod;

    private TraceContextAccessor() {}

    public static String traceId() {
        ensureLoaded();
        if (traceMethod == null) {
            return null;
        }
        try {
            Object value = traceMethod.invoke(null);
            return value == null ? null : value.toString();
        } catch (Exception ignored) {
            return null;
        }
    }

    public static String requestId() {
        ensureLoaded();
        if (requestMethod == null) {
            return null;
        }
        try {
            Object value = requestMethod.invoke(null);
            return value == null ? null : value.toString();
        } catch (Exception ignored) {
            return null;
        }
    }

    private static synchronized void ensureLoaded() {
        if (loaded) {
            return;
        }
        loaded = true;
        Class<?> traceClass = loadTraceContextClass();
        if (traceClass == null) {
            return;
        }
        traceMethod = ReflectionUtils.findStaticMethod(traceClass, "traceId", 0);
        requestMethod = ReflectionUtils.findStaticMethod(traceClass, "requestId", 0);
    }

    private static Class<?> loadTraceContextClass() {
        try {
            return Class.forName(TRACE_CONTEXT_CLASS, false, Thread.currentThread().getContextClassLoader());
        } catch (ClassNotFoundException ignored) {
            try {
                return Class.forName(TRACE_CONTEXT_CLASS, false, TraceContextAccessor.class.getClassLoader());
            } catch (ClassNotFoundException ignoredAgain) {
                return null;
            }
        }
    }
}
