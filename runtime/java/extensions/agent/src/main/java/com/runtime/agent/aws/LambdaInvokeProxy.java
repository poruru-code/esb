// Where: runtime/java/extensions/agent/src/main/java/com/runtime/agent/aws/LambdaInvokeProxy.java
// What: Direct Lambda invoke proxy to the Gateway when SDK calls need local routing.
// Why: Avoid outbound AWS calls while preserving Java SDK semantics.
package com.runtime.agent.aws;

import com.runtime.agent.util.ReflectionUtils;
import com.runtime.agent.util.TraceContextAccessor;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.lang.reflect.Method;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.atomic.AtomicBoolean;
import javax.net.ssl.HostnameVerifier;
import javax.net.ssl.HttpsURLConnection;
import javax.net.ssl.SSLContext;
import javax.net.ssl.TrustManager;
import javax.net.ssl.X509TrustManager;

public final class LambdaInvokeProxy {
    private static final AtomicBoolean TRUST_ALL_INSTALLED = new AtomicBoolean(false);
    private static final int CONNECT_TIMEOUT_MS = 3000;
    private static final int READ_TIMEOUT_MS = 15000;

    private LambdaInvokeProxy() {}

    public static Object handle(Method method, Object request) {
        if (method == null || request == null) {
            return null;
        }
        String endpoint = System.getenv("GATEWAY_INTERNAL_URL");
        if (endpoint == null || endpoint.isBlank()) {
            return null;
        }

        ensureTrustAll();

        InvokeResult result = invokeGateway(endpoint, request);
        if (result == null) {
            return null;
        }

        Object response = buildInvokeResponse(request, result.statusCode, result.payload);
        if (response == null) {
            return null;
        }

        if (CompletableFuture.class.isAssignableFrom(method.getReturnType())) {
            return CompletableFuture.completedFuture(response);
        }
        return response;
    }

    private static InvokeResult invokeGateway(String endpoint, Object request) {
        try {
            String functionName = asString(ReflectionUtils.invoke(request, "functionName"));
            if (functionName == null || functionName.isBlank()) {
                return null;
            }

            Object payloadObj = ReflectionUtils.invoke(request, "payload");
            byte[] payload = extractPayloadBytes(payloadObj);
            String target = buildInvokeUrl(endpoint, functionName);

            HttpURLConnection conn = (HttpURLConnection) new URL(target).openConnection();
            conn.setRequestMethod("POST");
            conn.setConnectTimeout(CONNECT_TIMEOUT_MS);
            conn.setReadTimeout(READ_TIMEOUT_MS);
            conn.setDoOutput(true);
            conn.setRequestProperty("Content-Type", "application/json");

            String clientContext = asString(ReflectionUtils.invoke(request, "clientContext"));
            if (clientContext != null && !clientContext.isBlank()) {
                conn.setRequestProperty("X-Amz-Client-Context", clientContext);
            }
            String traceId = TraceContextAccessor.traceId();
            if (traceId != null && !traceId.isBlank()) {
                conn.setRequestProperty("X-Amzn-Trace-Id", traceId);
            }

            if (payload != null && payload.length > 0) {
                try (OutputStream os = conn.getOutputStream()) {
                    os.write(payload);
                }
            }

            int status = conn.getResponseCode();
            byte[] responseBody = readAll(status < 400 ? conn.getInputStream() : conn.getErrorStream());
            return new InvokeResult(status, responseBody == null ? new byte[0] : responseBody);
        } catch (Exception ignored) {
            return null;
        }
    }

    private static String buildInvokeUrl(String endpoint, String functionName) {
        String base = endpoint.endsWith("/") ? endpoint.substring(0, endpoint.length() - 1) : endpoint;
        return base + "/2015-03-31/functions/" + functionName + "/invocations";
    }

    private static byte[] extractPayloadBytes(Object payloadObj) {
        if (payloadObj == null) {
            return new byte[0];
        }
        if (payloadObj instanceof byte[] bytes) {
            return bytes;
        }
        Object bytesObj = ReflectionUtils.invoke(payloadObj, "asByteArray");
        if (bytesObj instanceof byte[] bytes) {
            return bytes;
        }
        String text = asString(ReflectionUtils.invoke(payloadObj, "asUtf8String"));
        if (text != null) {
            return text.getBytes(StandardCharsets.UTF_8);
        }
        return new byte[0];
    }

    private static Object buildInvokeResponse(Object context, int statusCode, byte[] payload) {
        try {
            Class<?> sdkBytesClass = ReflectionUtils.loadClass(
                    "software.amazon.awssdk.core.SdkBytes",
                    context
            );
            Object sdkBytes = null;
            if (sdkBytesClass != null && payload != null) {
                sdkBytes = ReflectionUtils.invokeStatic(sdkBytesClass, "fromByteArray", payload);
            }
            Class<?> responseClass = ReflectionUtils.loadClass(
                    "software.amazon.awssdk.services.lambda.model.InvokeResponse",
                    context
            );
            if (responseClass == null) {
                return null;
            }
            Object builder = ReflectionUtils.invokeStatic(responseClass, "builder");
            if (builder == null) {
                return null;
            }
            ReflectionUtils.invoke(builder, "statusCode", statusCode);
            if (sdkBytes != null) {
                ReflectionUtils.invoke(builder, "payload", sdkBytes);
            }
            return ReflectionUtils.invoke(builder, "build");
        } catch (Exception ignored) {
            return null;
        }
    }

    private static byte[] readAll(InputStream in) {
        if (in == null) {
            return new byte[0];
        }
        try (InputStream input = in; ByteArrayOutputStream buffer = new ByteArrayOutputStream()) {
            byte[] chunk = new byte[4096];
            int read;
            while ((read = input.read(chunk)) >= 0) {
                buffer.write(chunk, 0, read);
            }
            return buffer.toByteArray();
        } catch (Exception ignored) {
            return new byte[0];
        }
    }

    private static String asString(Object value) {
        return value == null ? null : value.toString();
    }

    private static void ensureTrustAll() {
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

    private record InvokeResult(int statusCode, byte[] payload) {}
}
