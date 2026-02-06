// Where: runtime/java/extensions/agent/src/main/java/com/runtime/agent/aws/CloudWatchLogsRequestGuard.java
// What: Guard logic for CloudWatch Logs request argument shape.
// Why: Avoid skipping SDK calls for Consumer overloads and unsupported request shapes.
package com.runtime.agent.aws;

import java.lang.reflect.Method;
import java.util.function.Consumer;

public final class CloudWatchLogsRequestGuard {
    private static final String REQUEST_PACKAGE_PREFIX =
            "software.amazon.awssdk.services.cloudwatchlogs.model.";

    private CloudWatchLogsRequestGuard() {}

    public static boolean isSupported(Method method, Object request) {
        if (method == null || request == null) {
            return false;
        }

        Class<?>[] parameterTypes = method.getParameterTypes();
        if (parameterTypes.length == 0 || Consumer.class.isAssignableFrom(parameterTypes[0])) {
            return false;
        }

        String requestClassName = request.getClass().getName();
        return requestClassName.startsWith(REQUEST_PACKAGE_PREFIX) && requestClassName.endsWith("Request");
    }
}
