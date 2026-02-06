// Where: runtime/java/extensions/agent/src/main/java/com/runtime/agent/AgentMain.java
// What: Java agent entrypoint that installs AWS SDK hooks and log forwarding.
// Why: Replicate sitecustomize.py behavior for Java without application changes.
package com.runtime.agent;

import com.runtime.agent.advice.CloudWatchLogsAdvice;
import com.runtime.agent.advice.LambdaInvokeAdvice;
import com.runtime.agent.advice.SdkBuilderAdvice;
import com.runtime.agent.logging.VictoriaLogsHook;
import java.lang.instrument.Instrumentation;
import java.util.concurrent.atomic.AtomicBoolean;
import net.bytebuddy.agent.builder.AgentBuilder;
import net.bytebuddy.asm.Advice;
import net.bytebuddy.description.type.TypeDescription;
import net.bytebuddy.dynamic.DynamicType;
import net.bytebuddy.matcher.ElementMatchers;
import net.bytebuddy.utility.JavaModule;
import java.security.ProtectionDomain;

public final class AgentMain {
    private static final AtomicBoolean INSTALLED = new AtomicBoolean(false);

    private AgentMain() {}

    public static void premain(String agentArgs, Instrumentation instrumentation) {
        if (!INSTALLED.compareAndSet(false, true)) {
            return;
        }

        VictoriaLogsHook.install();

        AgentBuilder builder = new AgentBuilder.Default()
                .ignore(ElementMatchers.nameStartsWith("net.bytebuddy.")
                        .or(ElementMatchers.nameStartsWith("com.runtime.agent.")));

        builder = builder
                .type(ElementMatchers.hasSuperType(
                        ElementMatchers.named("software.amazon.awssdk.core.client.builder.SdkClientBuilder")))
                .transform(new AgentBuilder.Transformer() {
                    @Override
                    public DynamicType.Builder<?> transform(
                            DynamicType.Builder<?> b,
                            TypeDescription type,
                            ClassLoader classLoader,
                            JavaModule module,
                            ProtectionDomain protectionDomain
                    ) {
                        return b.visit(Advice.to(SdkBuilderAdvice.class)
                                .on(ElementMatchers.named("build").and(ElementMatchers.takesArguments(0))));
                    }
                });

        builder = builder
                .type(ElementMatchers.nameStartsWith("software.amazon.awssdk.services.lambda.")
                        .and(ElementMatchers.nameEndsWith("LambdaClient").or(ElementMatchers.nameEndsWith("Client")))
                        .and(ElementMatchers.not(ElementMatchers.isInterface())))
                .transform(new AgentBuilder.Transformer() {
                    @Override
                    public DynamicType.Builder<?> transform(
                            DynamicType.Builder<?> b,
                            TypeDescription type,
                            ClassLoader classLoader,
                            JavaModule module,
                            ProtectionDomain protectionDomain
                    ) {
                        return b.visit(Advice.to(LambdaInvokeAdvice.class)
                                .on(ElementMatchers.named("invoke").and(ElementMatchers.takesArguments(1))));
                    }
                });

        builder = builder
                .type(ElementMatchers.nameStartsWith("software.amazon.awssdk.services.lambda.")
                        .and(ElementMatchers.nameEndsWith("LambdaAsyncClient").or(ElementMatchers.nameEndsWith("AsyncClient")))
                        .and(ElementMatchers.not(ElementMatchers.isInterface())))
                .transform(new AgentBuilder.Transformer() {
                    @Override
                    public DynamicType.Builder<?> transform(
                            DynamicType.Builder<?> b,
                            TypeDescription type,
                            ClassLoader classLoader,
                            JavaModule module,
                            ProtectionDomain protectionDomain
                    ) {
                        return b.visit(Advice.to(LambdaInvokeAdvice.class)
                                .on(ElementMatchers.named("invoke").and(ElementMatchers.takesArguments(1))));
                    }
                });

        builder = builder
                .type(ElementMatchers.nameStartsWith("software.amazon.awssdk.services.cloudwatchlogs.")
                        .and(ElementMatchers.nameEndsWith("CloudWatchLogsClient").or(ElementMatchers.nameEndsWith("Client")))
                        .and(ElementMatchers.not(ElementMatchers.isInterface())))
                .transform(new AgentBuilder.Transformer() {
                    @Override
                    public DynamicType.Builder<?> transform(
                            DynamicType.Builder<?> b,
                            TypeDescription type,
                            ClassLoader classLoader,
                            JavaModule module,
                            ProtectionDomain protectionDomain
                    ) {
                        return b.visit(Advice.to(CloudWatchLogsAdvice.class)
                                .on(ElementMatchers.namedOneOf(
                                        "putLogEvents",
                                        "createLogGroup",
                                        "createLogStream",
                                        "deleteLogGroup",
                                        "deleteLogStream",
                                        "describeLogGroups",
                                        "describeLogStreams"
                                )));
                    }
                });

        builder = builder
                .type(ElementMatchers.nameStartsWith("software.amazon.awssdk.services.cloudwatchlogs.")
                        .and(ElementMatchers.nameEndsWith("CloudWatchLogsAsyncClient").or(ElementMatchers.nameEndsWith("AsyncClient")))
                        .and(ElementMatchers.not(ElementMatchers.isInterface())))
                .transform(new AgentBuilder.Transformer() {
                    @Override
                    public DynamicType.Builder<?> transform(
                            DynamicType.Builder<?> b,
                            TypeDescription type,
                            ClassLoader classLoader,
                            JavaModule module,
                            ProtectionDomain protectionDomain
                    ) {
                        return b.visit(Advice.to(CloudWatchLogsAdvice.class)
                                .on(ElementMatchers.namedOneOf(
                                        "putLogEvents",
                                        "createLogGroup",
                                        "createLogStream",
                                        "deleteLogGroup",
                                        "deleteLogStream",
                                        "describeLogGroups",
                                        "describeLogStreams"
                                )));
                    }
                });

        builder.installOn(instrumentation);
    }
}
