// Where: runtime/java/extensions/agent/src/test/java/com/runtime/agent/aws/CloudWatchLogsRequestGuardTest.java
// What: Unit tests for CloudWatch Logs request argument guard.
// Why: Prevent Consumer overloads from being treated as request-object invocations.
package com.runtime.agent.aws;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import java.lang.reflect.Method;
import java.util.function.Consumer;
import org.junit.Test;
import software.amazon.awssdk.services.cloudwatchlogs.model.FakeLogRequest;

public class CloudWatchLogsRequestGuardTest {
    private interface CloudWatchOverloadsShape {
        Object putLogEvents(Consumer<Object> requestBuilderConsumer);

        Object putLogEvents(Object request);
    }

    @Test
    public void returnsFalseWhenMethodOrRequestIsNull() {
        assertFalse(CloudWatchLogsRequestGuard.isSupported(null, new Object()));
        assertFalse(CloudWatchLogsRequestGuard.isSupported(dummyMethod(), null));
    }

    @Test
    public void returnsFalseForConsumerOverloadEvenWithRequestLikeObject() throws Exception {
        Method method = CloudWatchOverloadsShape.class.getMethod("putLogEvents", Consumer.class);
        assertFalse(CloudWatchLogsRequestGuard.isSupported(method, new FakeLogRequest()));
    }

    @Test
    public void returnsFalseForNonCloudWatchRequestType() throws Exception {
        Method method = CloudWatchOverloadsShape.class.getMethod("putLogEvents", Object.class);
        assertFalse(CloudWatchLogsRequestGuard.isSupported(method, new Object()));
    }

    @Test
    public void returnsTrueForCloudWatchRequestObject() throws Exception {
        Method method = CloudWatchOverloadsShape.class.getMethod("putLogEvents", Object.class);
        assertTrue(CloudWatchLogsRequestGuard.isSupported(method, new FakeLogRequest()));
    }

    private Method dummyMethod() {
        try {
            return Object.class.getMethod("toString");
        } catch (NoSuchMethodException e) {
            throw new IllegalStateException(e);
        }
    }
}
