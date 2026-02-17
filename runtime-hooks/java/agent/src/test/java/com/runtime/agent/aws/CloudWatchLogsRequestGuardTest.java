// Where: runtime-hooks/java/agent/src/test/java/com/runtime/agent/aws/CloudWatchLogsRequestGuardTest.java
// What: Unit tests for CloudWatch Logs request argument guard.
// Why: Allow only safe Consumer overloads while preventing unsupported request shapes.
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

        Object createLogGroup(Consumer<Object> requestBuilderConsumer);

        Object createLogStream(Consumer<Object> requestBuilderConsumer);

        Object deleteLogGroup(Consumer<Object> requestBuilderConsumer);

        Object deleteLogStream(Consumer<Object> requestBuilderConsumer);

        Object describeLogGroups(Consumer<Object> requestBuilderConsumer);

        Object describeLogStreams(Consumer<Object> requestBuilderConsumer);

        Object putLogEvents(Object request);
    }

    @Test
    public void returnsFalseWhenMethodOrRequestIsNull() {
        assertFalse(CloudWatchLogsRequestGuard.isSupported(null, new Object()));
        assertFalse(CloudWatchLogsRequestGuard.isSupported(dummyMethod(), null));
    }

    @Test
    public void returnsFalseForUnsupportedConsumerOverload() throws Exception {
        assertConsumerOverloadSupport("putLogEvents", false);
    }

    @Test
    public void returnsTrueForAllSafeConsumerOverloads() throws Exception {
        String[] supportedMethods = {
                "createLogGroup",
                "createLogStream",
                "deleteLogGroup",
                "deleteLogStream",
                "describeLogGroups",
                "describeLogStreams",
        };
        for (String methodName : supportedMethods) {
            assertConsumerOverloadSupport(methodName, true);
        }
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

    private void assertConsumerOverloadSupport(String methodName, boolean expected) throws Exception {
        Method method = CloudWatchOverloadsShape.class.getMethod(methodName, Consumer.class);
        Consumer<Object> noopConsumer = ignored -> {};
        boolean actual = CloudWatchLogsRequestGuard.isSupported(method, noopConsumer);
        if (expected) {
            assertTrue(actual);
            return;
        }
        assertFalse(actual);
    }
}
