// Where: runtime/java/extensions/agent/src/main/java/com/runtime/agent/advice/CloudWatchLogsAdvice.java
// What: Advice that replaces CloudWatch Logs calls with local handling.
// Why: Avoid outbound AWS calls and forward logs locally/VictoriaLogs.
package com.runtime.agent.advice;

import com.runtime.agent.aws.CloudWatchLogsMock;
import com.runtime.agent.aws.CloudWatchLogsRequestGuard;
import java.lang.reflect.Method;
import net.bytebuddy.asm.Advice;

public final class CloudWatchLogsAdvice {
    private CloudWatchLogsAdvice() {}

    @Advice.OnMethodEnter(skipOn = Advice.OnNonDefaultValue.class)
    public static Object onEnter(
            @Advice.Origin Method method,
            @Advice.AllArguments Object[] args
    ) {
        Object request = (args != null && args.length > 0) ? args[0] : null;
        if (!CloudWatchLogsRequestGuard.isSupported(method, request)) {
            return null;
        }
        return CloudWatchLogsMock.handle(method, request);
    }

    @Advice.OnMethodExit
    public static void onExit(
            @Advice.Enter Object response,
            @Advice.Return(readOnly = false) Object returned
    ) {
        if (response != null) {
            returned = response;
        }
    }
}
