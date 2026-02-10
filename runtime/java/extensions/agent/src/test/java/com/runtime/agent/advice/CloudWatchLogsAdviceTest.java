// Where: runtime/java/extensions/agent/src/test/java/com/runtime/agent/advice/CloudWatchLogsAdviceTest.java
// What: Unit tests for CloudWatch Logs advice interception behavior.
// Why: Ensure supported calls are always intercepted without falling back.
package com.runtime.agent.advice;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import java.lang.reflect.Method;
import java.util.function.Consumer;
import org.junit.Test;
import software.amazon.awssdk.services.cloudwatchlogs.model.FakeLogRequest;

public class CloudWatchLogsAdviceTest {
    private interface CloudWatchConsumerOverloadShape {
        Object putLogEvents(Consumer<Object> requestBuilderConsumer);

        Object createLogGroup(Consumer<Object> requestBuilderConsumer);

        Object putLogEvents(Object request);
    }

    @Test
    public void onEnterReturnsFalseWhenRequestIsUnsupported() throws Exception {
        Method unsupportedMethod = Object.class.getMethod("toString");
        boolean enterResult = CloudWatchLogsAdvice.onEnter(unsupportedMethod, new Object[0], null);
        assertFalse(enterResult);
    }

    @Test
    public void onEnterReturnsFalseForUnsupportedConsumerOverload() throws Exception {
        Method consumerOverload =
                CloudWatchConsumerOverloadShape.class.getMethod("putLogEvents", Consumer.class);
        Consumer<Object> noopConsumer = ignored -> {};
        boolean enterResult =
                CloudWatchLogsAdvice.onEnter(consumerOverload, new Object[] {noopConsumer}, null);
        assertFalse(enterResult);
    }

    @Test
    public void onEnterReturnsTrueForSupportedConsumerOverload() throws Exception {
        Method consumerOverload =
                CloudWatchConsumerOverloadShape.class.getMethod("createLogGroup", Consumer.class);
        Consumer<Object> noopConsumer = ignored -> {};
        boolean enterResult =
                CloudWatchLogsAdvice.onEnter(consumerOverload, new Object[] {noopConsumer}, null);
        assertTrue(enterResult);
    }

    @Test
    public void onEnterReturnsTrueForCloudWatchRequestObject() throws Exception {
        Method requestMethod = CloudWatchConsumerOverloadShape.class.getMethod("putLogEvents", Object.class);
        boolean enterResult =
                CloudWatchLogsAdvice.onEnter(requestMethod, new Object[] {new FakeLogRequest()}, null);
        assertTrue(enterResult);
    }
}
