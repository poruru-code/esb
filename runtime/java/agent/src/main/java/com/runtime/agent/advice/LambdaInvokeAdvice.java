// Where: runtime/java/agent/src/main/java/com/runtime/agent/advice/LambdaInvokeAdvice.java
// What: Advice that injects trace_id into Lambda Invoke ClientContext.
// Why: Keep trace propagation compatible with Python sitecustomize behavior.
package com.runtime.agent.advice;

import com.runtime.agent.aws.LambdaClientContextInjector;
import com.runtime.agent.aws.LambdaInvokeProxy;
import java.lang.reflect.Method;
import net.bytebuddy.asm.Advice;

public final class LambdaInvokeAdvice {
    private LambdaInvokeAdvice() {}

    @Advice.OnMethodEnter(skipOn = Advice.OnNonDefaultValue.class)
    public static Object onEnter(
            @Advice.Origin Method method,
            @Advice.Argument(value = 0, readOnly = false) Object request
    ) {
        Object updated = LambdaClientContextInjector.inject(request);
        if (updated != null) {
            request = updated;
        }
        return LambdaInvokeProxy.handle(method, request);
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
