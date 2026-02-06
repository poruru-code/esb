// Where: runtime/java/extensions/agent/src/main/java/com/runtime/agent/advice/CloudWatchLogsAdvice.java
// What: Advice that replaces CloudWatch Logs calls with local handling.
// Why: Avoid outbound AWS calls and forward logs locally/VictoriaLogs.
package com.runtime.agent.advice;

import com.runtime.agent.aws.CloudWatchLogsMock;
import java.lang.reflect.Method;
import net.bytebuddy.asm.Advice;

public final class CloudWatchLogsAdvice {
    private static final Object SKIP = new Object();

    private CloudWatchLogsAdvice() {}

    @Advice.OnMethodEnter(skipOn = Advice.OnNonDefaultValue.class)
    public static Object onEnter(
            @Advice.Origin Method method,
            @Advice.AllArguments Object[] args
    ) {
        Object request = (args != null && args.length > 0) ? args[0] : null;
        Object response = CloudWatchLogsMock.handle(method, request);
        return response != null ? response : SKIP;
    }

    @Advice.OnMethodExit
    public static void onExit(
            @Advice.Enter Object response,
            @Advice.Return(readOnly = false) Object returned
    ) {
        if (response != null && response != SKIP) {
            returned = response;
        }
    }
}
