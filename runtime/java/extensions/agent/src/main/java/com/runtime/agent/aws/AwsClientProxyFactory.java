// Where: runtime/java/extensions/agent/src/main/java/com/runtime/agent/aws/AwsClientProxyFactory.java
// What: Wrap AWS SDK clients to intercept Lambda/CloudWatch calls at interface level.
// Why: Ensure local routing even when Byte Buddy type transforms miss client classes.
package com.runtime.agent.aws;

import java.lang.reflect.InvocationHandler;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.lang.reflect.Proxy;
import java.util.Arrays;
import java.util.LinkedHashSet;
import java.util.Set;

public final class AwsClientProxyFactory {
    private AwsClientProxyFactory() {}

    public static Object wrap(Object builder, Object client) {
        if (client == null) {
            return null;
        }
        if (Proxy.isProxyClass(client.getClass())) {
            return client;
        }
        Service service = Service.from(builder, client);
        return switch (service) {
            case LAMBDA -> wrapLambda(client);
            case CLOUDWATCH_LOGS -> wrapCloudWatchLogs(client);
            default -> client;
        };
    }

    private static Object wrapLambda(Object client) {
        return createProxy(client, new LambdaHandler(client));
    }

    private static Object wrapCloudWatchLogs(Object client) {
        return createProxy(client, new CloudWatchHandler(client));
    }

    private static Object createProxy(Object client, InvocationHandler handler) {
        Class<?>[] interfaces = collectInterfaces(client.getClass());
        if (interfaces.length == 0) {
            return client;
        }
        return Proxy.newProxyInstance(client.getClass().getClassLoader(), interfaces, handler);
    }

    private static Class<?>[] collectInterfaces(Class<?> type) {
        Set<Class<?>> interfaces = new LinkedHashSet<>();
        Class<?> current = type;
        while (current != null) {
            interfaces.addAll(Arrays.asList(current.getInterfaces()));
            current = current.getSuperclass();
        }
        return interfaces.toArray(new Class<?>[0]);
    }

    private static Object invokeOriginal(Object target, Method method, Object[] args) throws Throwable {
        try {
            return method.invoke(target, args);
        } catch (InvocationTargetException e) {
            throw e.getCause();
        }
    }

    private static boolean isObjectMethod(Method method) {
        return method.getDeclaringClass() == Object.class;
    }

    private static final class LambdaHandler implements InvocationHandler {
        private final Object target;

        private LambdaHandler(Object target) {
            this.target = target;
        }

        @Override
        public Object invoke(Object proxy, Method method, Object[] args) throws Throwable {
            if (isObjectMethod(method)) {
                return invokeOriginal(target, method, args);
            }
            if ("invoke".equals(method.getName()) && args != null && args.length == 1) {
                Object request = args[0];
                Object updated = LambdaClientContextInjector.inject(request);
                if (updated != null) {
                    args[0] = updated;
                    request = updated;
                }
                Object handled = LambdaInvokeProxy.handle(method, request);
                if (handled != null) {
                    return handled;
                }
            }
            return invokeOriginal(target, method, args);
        }
    }

    private static final class CloudWatchHandler implements InvocationHandler {
        private final Object target;

        private CloudWatchHandler(Object target) {
            this.target = target;
        }

        @Override
        public Object invoke(Object proxy, Method method, Object[] args) throws Throwable {
            if (isObjectMethod(method)) {
                return invokeOriginal(target, method, args);
            }
            if (isCloudWatchMethod(method)) {
                Object request = (args != null && args.length > 0) ? args[0] : null;
                Object response = CloudWatchLogsMock.handle(method, request);
                if (response != null) {
                    return response;
                }
            }
            return invokeOriginal(target, method, args);
        }

        private boolean isCloudWatchMethod(Method method) {
            String name = method.getName();
            return name.equals("putLogEvents")
                    || name.equals("createLogGroup")
                    || name.equals("createLogStream")
                    || name.equals("deleteLogGroup")
                    || name.equals("deleteLogStream")
                    || name.equals("describeLogGroups")
                    || name.equals("describeLogStreams");
        }
    }

    private enum Service {
        LAMBDA,
        CLOUDWATCH_LOGS,
        UNKNOWN;

        static Service from(Object builder, Object client) {
            Service fromBuilder = fromClassName(builder == null ? null : builder.getClass().getName());
            if (fromBuilder != UNKNOWN) {
                return fromBuilder;
            }
            return fromClassName(client == null ? null : client.getClass().getName());
        }

        static Service fromClassName(String className) {
            if (className == null) {
                return UNKNOWN;
            }
            if (className.contains(".lambda.")) {
                return LAMBDA;
            }
            if (className.contains(".cloudwatchlogs.")) {
                return CLOUDWATCH_LOGS;
            }
            return UNKNOWN;
        }
    }
}
