// Where: runtime/java/agent/src/main/java/com/runtime/agent/advice/SdkBuilderAdvice.java
// What: Byte Buddy advice to configure AWS SDK builders before build().
// Why: Inject endpoint overrides and service configs without app changes.
package com.runtime.agent.advice;

import com.runtime.agent.aws.AwsClientConfigurer;
import com.runtime.agent.aws.AwsClientProxyFactory;
import net.bytebuddy.asm.Advice;

public final class SdkBuilderAdvice {
    private SdkBuilderAdvice() {}

    @Advice.OnMethodEnter
    public static void onEnter(@Advice.This Object builder) {
        AwsClientConfigurer.configure(builder);
    }

    @Advice.OnMethodExit
    public static void onExit(
            @Advice.This Object builder,
            @Advice.Return(readOnly = false) Object returned
    ) {
        Object wrapped = AwsClientProxyFactory.wrap(builder, returned);
        if (wrapped != null) {
            returned = wrapped;
        }
    }
}
