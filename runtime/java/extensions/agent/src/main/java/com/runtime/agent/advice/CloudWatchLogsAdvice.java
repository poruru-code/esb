// Where: runtime/java/extensions/agent/src/main/java/com/runtime/agent/advice/CloudWatchLogsAdvice.java
// What: Advice that replaces CloudWatch Logs calls with local handling.
// Why: Avoid outbound AWS calls and forward logs locally/VictoriaLogs.
package com.runtime.agent.advice;

import com.runtime.agent.aws.CloudWatchLogsMock;
import com.runtime.agent.aws.CloudWatchLogsRequestGuard;
import java.lang.reflect.Method;
import net.bytebuddy.asm.Advice;
import net.bytebuddy.implementation.bytecode.assign.Assigner;

public final class CloudWatchLogsAdvice {
    private CloudWatchLogsAdvice() {}

    @Advice.OnMethodEnter(skipOn = Advice.OnNonDefaultValue.class)
    public static boolean onEnter(
            @Advice.Origin Method method,
            @Advice.AllArguments Object[] args,
            @Advice.Local("mockResponse") Object mockResponse
    ) {
        Object request = (args != null && args.length > 0) ? args[0] : null;
        if (!CloudWatchLogsRequestGuard.isSupported(method, request)) {
            mockResponse = null;
            return false;
        }
        mockResponse = CloudWatchLogsMock.handle(method, request);
        return true;
    }

    @Advice.OnMethodExit
    public static void onExit(
            @Advice.Enter boolean intercepted,
            @Advice.Local("mockResponse") Object mockResponse,
            @Advice.Return(readOnly = false, typing = Assigner.Typing.DYNAMIC) Object returned
    ) {
        if (intercepted) {
            returned = mockResponse;
        }
    }
}
