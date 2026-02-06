// Where: runtime/java/extensions/agent/src/test/java/com/runtime/agent/advice/CloudWatchLogsAdviceTest.java
// What: Unit test for CloudWatch Logs advice fallback behavior.
// Why: Ensure unsupported requests fall back to the original AWS SDK call.
package com.runtime.agent.advice;

import static org.junit.Assert.assertNull;

import java.lang.reflect.Method;
import java.util.function.Consumer;
import org.junit.Test;

public class CloudWatchLogsAdviceTest {
    private interface CloudWatchConsumerOverloadShape {
        Object putLogEvents(Consumer<Object> requestBuilderConsumer);
    }

    @Test
    public void onEnterReturnsNullWhenMockCannotHandleRequest() throws Exception {
        Method unsupportedMethod = Object.class.getMethod("toString");
        Object enterResult = CloudWatchLogsAdvice.onEnter(unsupportedMethod, new Object[0]);
        assertNull(enterResult);
    }

    @Test
    public void onEnterReturnsNullForConsumerOverload() throws Exception {
        Method consumerOverload =
                CloudWatchConsumerOverloadShape.class.getMethod("putLogEvents", Consumer.class);
        Consumer<Object> noopConsumer = ignored -> {};
        Object enterResult = CloudWatchLogsAdvice.onEnter(consumerOverload, new Object[] {noopConsumer});
        assertNull(enterResult);
    }
}
