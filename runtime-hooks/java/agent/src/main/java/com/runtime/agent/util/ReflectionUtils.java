// Where: runtime-hooks/java/agent/src/main/java/com/runtime/agent/util/ReflectionUtils.java
// What: Reflection helpers for best-effort SDK interaction.
// Why: Avoid compile-time dependency on specific AWS SDK versions.
package com.runtime.agent.util;

import java.lang.reflect.Field;
import java.lang.reflect.Method;

public final class ReflectionUtils {
    private ReflectionUtils() {}

    public static Class<?> loadClass(String className, Object context) {
        if (className == null || className.isEmpty()) {
            return null;
        }
        ClassLoader loader = resolveClassLoader(context);
        try {
            if (loader != null) {
                return Class.forName(className, false, loader);
            }
            return Class.forName(className);
        } catch (ClassNotFoundException ignored) {
            ClassLoader fallback = Thread.currentThread().getContextClassLoader();
            if (fallback != null && fallback != loader) {
                try {
                    return Class.forName(className, false, fallback);
                } catch (ClassNotFoundException ignoredAgain) {
                    return null;
                }
            }
            return null;
        }
    }

    private static ClassLoader resolveClassLoader(Object context) {
        if (context == null) {
            return null;
        }
        if (context instanceof Class<?> type) {
            return type.getClassLoader();
        }
        return context.getClass().getClassLoader();
    }

    public static Method findMethod(Object target, String name, int paramCount) {
        if (target == null) {
            return null;
        }
        for (Method method : target.getClass().getMethods()) {
            if (method.getName().equals(name) && method.getParameterCount() == paramCount) {
                method.setAccessible(true);
                return method;
            }
        }
        return null;
    }

    public static Method findStaticMethod(Class<?> type, String name, int paramCount) {
        if (type == null) {
            return null;
        }
        for (Method method : type.getMethods()) {
            if (method.getName().equals(name) && method.getParameterCount() == paramCount) {
                method.setAccessible(true);
                return method;
            }
        }
        return null;
    }

    public static Object invoke(Object target, String name, Object... args) {
        if (target == null) {
            return null;
        }
        int paramCount = args == null ? 0 : args.length;
        for (Method method : target.getClass().getMethods()) {
            if (!method.getName().equals(name) || method.getParameterCount() != paramCount) {
                continue;
            }
            try {
                method.setAccessible(true);
                return method.invoke(target, args);
            } catch (Exception ignored) {
                // try next overload
            }
        }
        return null;
    }

    public static Object invokeStatic(Class<?> type, String name, Object... args) {
        if (type == null) {
            return null;
        }
        int paramCount = args == null ? 0 : args.length;
        for (Method method : type.getMethods()) {
            if (!method.getName().equals(name) || method.getParameterCount() != paramCount) {
                continue;
            }
            if (!java.lang.reflect.Modifier.isStatic(method.getModifiers())) {
                continue;
            }
            try {
                method.setAccessible(true);
                return method.invoke(null, args);
            } catch (Exception ignored) {
                // try next overload
            }
        }
        return null;
    }

    public static Object readStaticField(Class<?> type, String fieldName) {
        if (type == null) {
            return null;
        }
        try {
            Field field = type.getField(fieldName);
            field.setAccessible(true);
            return field.get(null);
        } catch (Exception ignored) {
            return null;
        }
    }
}
