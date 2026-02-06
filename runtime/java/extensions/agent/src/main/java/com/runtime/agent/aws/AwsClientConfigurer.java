// Where: runtime/java/extensions/agent/src/main/java/com/runtime/agent/aws/AwsClientConfigurer.java
// What: Applies endpoint overrides and client configuration to AWS SDK builders.
// Why: Redirect SDK calls to compatible services without app changes.
package com.runtime.agent.aws;

import com.runtime.agent.util.ReflectionUtils;
import java.net.URI;
import java.time.Duration;
import java.util.concurrent.atomic.AtomicBoolean;
import javax.net.ssl.HostnameVerifier;
import javax.net.ssl.HttpsURLConnection;
import javax.net.ssl.SSLContext;
import javax.net.ssl.TrustManager;
import javax.net.ssl.X509TrustManager;

public final class AwsClientConfigurer {
    private static final String ENV_S3_ENDPOINT = "S3_ENDPOINT";
    private static final String ENV_S3_ENDPOINT_ALT = "AWS_ENDPOINT_URL_S3";
    private static final String ENV_DYNAMODB_ENDPOINT = "DYNAMODB_ENDPOINT";
    private static final String ENV_DYNAMODB_ENDPOINT_ALT = "AWS_ENDPOINT_URL_DYNAMODB";
    private static final String ENV_LAMBDA_ENDPOINT = "GATEWAY_INTERNAL_URL";

    private static final Duration ATTEMPT_TIMEOUT = Duration.ofSeconds(5);
    private static final Duration CALL_TIMEOUT = Duration.ofSeconds(10);
    private static final AtomicBoolean TRUST_ALL_INSTALLED = new AtomicBoolean(false);

    private AwsClientConfigurer() {}

    public static void configure(Object builder) {
        if (builder == null) {
            return;
        }

        String className = builder.getClass().getName();
        Service service = Service.fromClassName(className);
        if (service == Service.UNKNOWN) {
            return;
        }

        if (service == Service.LAMBDA) {
            if (!applyUrlConnectionHttpClient(builder)) {
                applyApacheTrustAllHttpClient(builder);
            }
        }

        String endpoint = resolveEndpoint(service);
        if (endpoint != null && !endpoint.isEmpty()) {
            applyEndpointOverride(builder, endpoint);
            applyTrustAll(builder);
        }

        if (service == Service.S3) {
            applyS3PathStyle(builder);
        }

        if (service == Service.DYNAMODB || service == Service.LAMBDA) {
            applyRetryAndTimeouts(builder);
        }
    }

    private static String resolveEndpoint(Service service) {
        return switch (service) {
            case S3 -> firstEnv(ENV_S3_ENDPOINT, ENV_S3_ENDPOINT_ALT);
            case DYNAMODB -> firstEnv(ENV_DYNAMODB_ENDPOINT, ENV_DYNAMODB_ENDPOINT_ALT);
            case LAMBDA -> firstEnv(ENV_LAMBDA_ENDPOINT);
            default -> null;
        };
    }

    private static String firstEnv(String... keys) {
        for (String key : keys) {
            String value = System.getenv(key);
            if (value != null && !value.trim().isEmpty()) {
                return value.trim();
            }
        }
        return null;
    }

    private static void applyEndpointOverride(Object builder, String endpoint) {
        try {
            URI uri = URI.create(endpoint);
            ReflectionUtils.invoke(builder, "endpointOverride", uri);
        } catch (Exception ignored) {
            // best effort
        }
    }

    private static void applyS3PathStyle(Object builder) {
        try {
            if (ReflectionUtils.invoke(builder, "forcePathStyle", true) != null) {
                return;
            }
            if (ReflectionUtils.invoke(builder, "pathStyleAccessEnabled", true) != null) {
                return;
            }

            Class<?> cfgClass = ReflectionUtils.loadClass(
                    "software.amazon.awssdk.services.s3.S3Configuration",
                    builder
            );
            if (cfgClass == null) {
                return;
            }
            Object cfgBuilder = ReflectionUtils.invokeStatic(cfgClass, "builder");
            if (cfgBuilder != null) {
                ReflectionUtils.invoke(cfgBuilder, "pathStyleAccessEnabled", true);
                Object config = ReflectionUtils.invoke(cfgBuilder, "build");
                if (config != null && ReflectionUtils.invoke(builder, "serviceConfiguration", config) != null) {
                    return;
                }
                java.util.function.Consumer<Object> consumer = (Object b) -> {
                    ReflectionUtils.invoke(b, "pathStyleAccessEnabled", true);
                };
                ReflectionUtils.invoke(builder, "serviceConfiguration", consumer);
            }
        } catch (Exception ignored) {
            // best effort
        }
    }

    private static void applyRetryAndTimeouts(Object builder) {
        try {
            Object overrideConfig = buildOverrideConfiguration(builder);
            if (overrideConfig != null) {
                ReflectionUtils.invoke(builder, "overrideConfiguration", overrideConfig);
            }
        } catch (Exception ignored) {
            // best effort
        }
    }

    private static boolean applyUrlConnectionHttpClient(Object builder) {
        try {
            Class<?> clientClass = ReflectionUtils.loadClass(
                    "software.amazon.awssdk.http.urlconnection.UrlConnectionHttpClient",
                    builder
            );
            if (clientClass == null) {
                return false;
            }
            Object clientBuilder = ReflectionUtils.invokeStatic(clientClass, "builder");
            if (clientBuilder == null) {
                return false;
            }
            if (ReflectionUtils.invoke(builder, "httpClientBuilder", clientBuilder) != null) {
                return true;
            }
            Object client = ReflectionUtils.invoke(clientBuilder, "build");
            if (client != null) {
                ReflectionUtils.invoke(builder, "httpClient", client);
                return true;
            }
        } catch (Exception ignored) {
            // best effort
        }
        return false;
    }

    private static boolean applyApacheTrustAllHttpClient(Object builder) {
        try {
            Class<?> apacheClientClass = ReflectionUtils.loadClass(
                    "software.amazon.awssdk.http.apache.ApacheHttpClient",
                    builder
            );
            if (apacheClientClass == null) {
                return false;
            }
            Object apacheBuilder = ReflectionUtils.invokeStatic(apacheClientClass, "builder");
            if (apacheBuilder == null) {
                return false;
            }
            Class<?> tlsProviderClass = ReflectionUtils.loadClass(
                    "software.amazon.awssdk.http.TlsTrustManagersProvider",
                    builder
            );
            if (tlsProviderClass != null) {
                Object proxy = java.lang.reflect.Proxy.newProxyInstance(
                        tlsProviderClass.getClassLoader(),
                        new Class<?>[]{tlsProviderClass},
                        (obj, method, args) -> {
                            if ("trustManagers".equals(method.getName())) {
                                return new TrustManager[]{new TrustAllManager()};
                            }
                            return null;
                        }
                );
                ReflectionUtils.invoke(apacheBuilder, "tlsTrustManagersProvider", proxy);
            }

            if (ReflectionUtils.invoke(builder, "httpClientBuilder", apacheBuilder) != null) {
                return true;
            }
            Object client = ReflectionUtils.invoke(apacheBuilder, "build");
            if (client != null) {
                ReflectionUtils.invoke(builder, "httpClient", client);
                return true;
            }
        } catch (Exception ignored) {
            // best effort
        }
        return false;
    }

    private static Object buildOverrideConfiguration(Object context) {
        try {
            Class<?> overrideClass = ReflectionUtils.loadClass(
                    "software.amazon.awssdk.core.client.config.ClientOverrideConfiguration",
                    context
            );
            if (overrideClass == null) {
                return null;
            }
            Object overrideBuilder = ReflectionUtils.invokeStatic(overrideClass, "builder");
            if (overrideBuilder == null) {
                return null;
            }

            applyRetryPolicy(overrideBuilder, context);
            ReflectionUtils.invoke(overrideBuilder, "apiCallAttemptTimeout", ATTEMPT_TIMEOUT);
            ReflectionUtils.invoke(overrideBuilder, "apiCallTimeout", CALL_TIMEOUT);
            if (!applyTrustAllAdvancedOption(overrideBuilder, context)) {
                ensureGlobalTrustAll();
            }

            return ReflectionUtils.invoke(overrideBuilder, "build");
        } catch (Exception ignored) {
            return null;
        }
    }

    private static void applyRetryPolicy(Object overrideBuilder, Object context) {
        try {
            Class<?> retryPolicyClass = ReflectionUtils.loadClass(
                    "software.amazon.awssdk.core.retry.RetryPolicy",
                    context
            );
            if (retryPolicyClass == null) {
                return;
            }
            Object retryBuilder = ReflectionUtils.invokeStatic(retryPolicyClass, "builder");
            if (retryBuilder == null) {
                return;
            }
            ReflectionUtils.invoke(retryBuilder, "numRetries", 9);
            applyRetryMode(retryBuilder, context);
            Object retryPolicy = ReflectionUtils.invoke(retryBuilder, "build");
            if (retryPolicy != null) {
                ReflectionUtils.invoke(overrideBuilder, "retryPolicy", retryPolicy);
            }
        } catch (Exception ignored) {
            // best effort
        }
    }

    private static void applyRetryMode(Object retryBuilder, Object context) {
        try {
            Class<?> retryModeClass = ReflectionUtils.loadClass(
                    "software.amazon.awssdk.core.retry.RetryMode",
                    context
            );
            if (retryModeClass == null) {
                return;
            }
            @SuppressWarnings("unchecked")
            Object retryMode = Enum.valueOf((Class<Enum>) retryModeClass.asSubclass(Enum.class), "STANDARD");
            ReflectionUtils.invoke(retryBuilder, "retryMode", retryMode);
        } catch (Exception ignored) {
            // best effort
        }
    }

    private static boolean applyTrustAllAdvancedOption(Object overrideBuilder, Object context) {
        try {
            Class<?> advOptionClass = ReflectionUtils.loadClass(
                    "software.amazon.awssdk.core.client.config.SdkAdvancedClientOption",
                    context
            );
            if (advOptionClass == null) {
                return false;
            }
            Object trustAll = ReflectionUtils.readStaticField(advOptionClass, "TRUST_ALL_CERTIFICATES");
            if (trustAll == null) {
                return false;
            }
            ReflectionUtils.invoke(overrideBuilder, "putAdvancedOption", trustAll, Boolean.TRUE);
            return true;
        } catch (Exception ignored) {
            return false;
        }
    }

    private static void applyTrustAll(Object builder) {
        if (builder == null) {
            return;
        }
        if (applyTrustAllViaOverride(builder)) {
            return;
        }
        ensureGlobalTrustAll();
    }

    private static boolean applyTrustAllViaOverride(Object builder) {
        try {
            Class<?> advOptionClass = ReflectionUtils.loadClass(
                    "software.amazon.awssdk.core.client.config.SdkAdvancedClientOption",
                    builder
            );
            Object trustAll = ReflectionUtils.readStaticField(advOptionClass, "TRUST_ALL_CERTIFICATES");
            if (trustAll == null) {
                return false;
            }
            Class<?> overrideClass = ReflectionUtils.loadClass(
                    "software.amazon.awssdk.core.client.config.ClientOverrideConfiguration",
                    builder
            );
            Object overrideBuilder = ReflectionUtils.invokeStatic(overrideClass, "builder");
            if (overrideBuilder == null) {
                return false;
            }
            ReflectionUtils.invoke(overrideBuilder, "putAdvancedOption", trustAll, Boolean.TRUE);
            Object overrideConfig = ReflectionUtils.invoke(overrideBuilder, "build");
            if (overrideConfig == null) {
                return false;
            }
            ReflectionUtils.invoke(builder, "overrideConfiguration", overrideConfig);
            return true;
        } catch (Exception ignored) {
            return false;
        }
    }

    private static void ensureGlobalTrustAll() {
        if (TRUST_ALL_INSTALLED.compareAndSet(false, true)) {
            installGlobalTrustAll();
        }
    }

    private static void installGlobalTrustAll() {
        try {
            TrustManager[] trustManagers = new TrustManager[]{new TrustAllManager()};
            SSLContext sslContext = SSLContext.getInstance("TLS");
            sslContext.init(null, trustManagers, new java.security.SecureRandom());
            SSLContext.setDefault(sslContext);
            HttpsURLConnection.setDefaultSSLSocketFactory(sslContext.getSocketFactory());
            HostnameVerifier verifier = (hostname, session) -> true;
            HttpsURLConnection.setDefaultHostnameVerifier(verifier);
        } catch (Exception ignored) {
            // best effort
        }
    }

    private enum Service {
        S3,
        DYNAMODB,
        LAMBDA,
        CLOUDWATCH_LOGS,
        UNKNOWN;

        static Service fromClassName(String className) {
            if (className == null) {
                return UNKNOWN;
            }
            if (className.contains(".s3.")) {
                return S3;
            }
            if (className.contains(".dynamodb.")) {
                return DYNAMODB;
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

    private static final class TrustAllManager implements X509TrustManager {
        @Override
        public void checkClientTrusted(java.security.cert.X509Certificate[] chain, String authType) {}

        @Override
        public void checkServerTrusted(java.security.cert.X509Certificate[] chain, String authType) {}

        @Override
        public java.security.cert.X509Certificate[] getAcceptedIssuers() {
            return new java.security.cert.X509Certificate[0];
        }
    }
}
