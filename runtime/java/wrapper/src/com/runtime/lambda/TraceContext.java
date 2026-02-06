// Where: runtime/java/wrapper/src/com/runtime/lambda/TraceContext.java
// What: Thread-local storage for trace and request identifiers.
// Why: Share trace context between the wrapper and javaagent without app changes.
package com.runtime.lambda;

public final class TraceContext {
    private static final ThreadLocal<String> TRACE_ID = new ThreadLocal<>();
    private static final ThreadLocal<String> REQUEST_ID = new ThreadLocal<>();

    private TraceContext() {}

    public static void set(String traceId, String requestId) {
        TRACE_ID.set(traceId);
        REQUEST_ID.set(requestId);
    }

    public static String traceId() {
        return TRACE_ID.get();
    }

    public static String requestId() {
        return REQUEST_ID.get();
    }

    public static void clear() {
        TRACE_ID.remove();
        REQUEST_ID.remove();
    }
}
