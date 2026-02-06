// Where: runtime/java/extensions/agent/src/test/java/com/runtime/agent/advice/CloudWatchLogsAdviceTest.java
// What: Unit test for CloudWatch Logs advice fallback behavior.
// Why: Ensure unsupported requests fall back to the original AWS SDK call.
package com.runtime.agent.advice;

import static org.junit.Assert.assertNull;

import java.lang.reflect.Method;
import org.junit.Test;

public class CloudWatchLogsAdviceTest {
    @Test
    public void onEnterReturnsNullWhenMockCannotHandleRequest() throws Exception {
        Method unsupportedMethod = Object.class.getMethod("toString");
        Object enterResult = CloudWatchLogsAdvice.onEnter(unsupportedMethod, new Object[0]);
        assertNull(enterResult);
    }
}
