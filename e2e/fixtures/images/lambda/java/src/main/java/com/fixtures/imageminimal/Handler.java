// Where: e2e/fixtures/images/lambda/java/src/main/java/com/fixtures/imageminimal/Handler.java
// What: Java image fixture Lambda handler for E2E image-function tests.
// Why: Validate Java image patch behavior with the same functional contract as Python fixture.
package com.fixtures.imageminimal;

import com.amazonaws.services.lambda.runtime.Context;
import com.amazonaws.services.lambda.runtime.RequestHandler;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import software.amazon.awssdk.core.ResponseBytes;
import software.amazon.awssdk.core.SdkBytes;
import software.amazon.awssdk.core.sync.RequestBody;
import software.amazon.awssdk.regions.Region;
import software.amazon.awssdk.services.cloudwatchlogs.CloudWatchLogsClient;
import software.amazon.awssdk.services.cloudwatchlogs.model.InputLogEvent;
import software.amazon.awssdk.services.cloudwatchlogs.model.PutLogEventsRequest;
import software.amazon.awssdk.services.lambda.LambdaClient;
import software.amazon.awssdk.services.lambda.model.InvocationType;
import software.amazon.awssdk.services.lambda.model.InvokeRequest;
import software.amazon.awssdk.services.lambda.model.InvokeResponse;
import software.amazon.awssdk.services.s3.S3Client;
import software.amazon.awssdk.services.s3.model.CreateBucketRequest;
import software.amazon.awssdk.services.s3.model.GetObjectRequest;
import software.amazon.awssdk.services.s3.model.PutObjectRequest;

public final class Handler implements RequestHandler<Map<String, Object>, Map<String, Object>> {
    private static final ObjectMapper MAPPER = new ObjectMapper().findAndRegisterModules();
    private static final TypeReference<Map<String, Object>> MAP_TYPE = new TypeReference<>() {};

    private static final String DEFAULT_BUCKET = "e2e-test-bucket";
    private static final String DEFAULT_TARGET = "lambda-echo";
    private static final String DEFAULT_REGION = "ap-northeast-1";

    @Override
    public Map<String, Object> handleRequest(Map<String, Object> event, Context context) {
        Map<String, Object> payload = parsePayload(event);
        String action = readString(payload, "action", "echo");

        Map<String, Object> response = new LinkedHashMap<>();
        response.put("success", true);
        response.put("action", action);
        response.put("handler", "esb-e2e-lambda-java");

        try {
            switch (action) {
                case "chain_invoke":
                    response.put("chain", invokeLambda(payload));
                    break;
                case "s3_roundtrip":
                    response.put("s3", s3Roundtrip(payload));
                    break;
                case "test_cloudwatch":
                    String marker = readString(payload, "marker", "image-cloudwatch-" + UUID.randomUUID());
                    response.put("cloudwatch", writeCloudWatchLog(marker));
                    emitMarker(marker);
                    break;
                default:
                    response.put("message", extractMessage(payload));
                    break;
            }
        } catch (Exception e) {
            Map<String, Object> error = new LinkedHashMap<>();
            error.put("success", false);
            error.put("action", action);
            error.put("handler", "esb-e2e-lambda-java");
            error.put("error", String.valueOf(e));
            return error;
        }

        return response;
    }

    private static Map<String, Object> parsePayload(Map<String, Object> event) {
        Map<String, Object> payload = new LinkedHashMap<>();
        if (event == null) {
            return payload;
        }

        Object body = event.get("body");
        if (body instanceof Map<?, ?> mapBody) {
            payload.putAll(castMap(mapBody));
        } else if (body instanceof String textBody) {
            payload.putAll(parseJsonMap(textBody));
        }

        for (String key : List.of("action", "message", "target", "bucket", "key", "content", "marker")) {
            if (payload.containsKey(key)) {
                continue;
            }
            Object value = event.get(key);
            if (value instanceof String strValue && !strValue.isBlank()) {
                payload.put(key, strValue);
            }
        }
        if (!payload.containsKey("async") && event.get("async") instanceof Boolean asyncValue) {
            payload.put("async", asyncValue);
        }
        return payload;
    }

    private static String extractMessage(Map<String, Object> payload) {
        return readString(payload, "message", "hello-image");
    }

    private static Map<String, Object> invokeLambda(Map<String, Object> payload) throws Exception {
        String target = readString(payload, "target", DEFAULT_TARGET);
        boolean async = payload.get("async") instanceof Boolean b && b;
        InvocationType invokeType = async ? InvocationType.EVENT : InvocationType.REQUEST_RESPONSE;
        String message = extractMessage(payload);

        Map<String, Object> requestPayload = new LinkedHashMap<>();
        requestPayload.put("message", message);
        String jsonPayload = MAPPER.writeValueAsString(requestPayload);

        InvokeRequest request = InvokeRequest.builder()
                .functionName(target)
                .invocationType(invokeType)
                .payload(SdkBytes.fromUtf8String(jsonPayload))
                .build();

        InvokeResponse invokeResponse;
        try (LambdaClient lambda = LambdaClient.builder().region(resolveRegion()).build()) {
            invokeResponse = lambda.invoke(request);
        }

        Map<String, Object> result = new LinkedHashMap<>();
        result.put("target", target);
        result.put("invocation_type", async ? "Event" : "RequestResponse");
        result.put("status_code", invokeResponse.statusCode());

        if (!async && invokeResponse.payload() != null) {
            String raw = invokeResponse.payload().asUtf8String();
            Map<String, Object> child = parseJsonMap(raw);
            result.put("child", child.isEmpty() ? raw : child);
        }
        return result;
    }

    private static Map<String, Object> s3Roundtrip(Map<String, Object> payload) {
        String bucket = readString(payload, "bucket", DEFAULT_BUCKET);
        String key = readString(payload, "key", "image-" + UUID.randomUUID().toString().substring(0, 8) + ".txt");
        String content = readString(payload, "content", "from-image-s3");

        try (S3Client s3 = S3Client.builder().region(resolveRegion()).build()) {
            try {
                s3.createBucket(CreateBucketRequest.builder().bucket(bucket).build());
            } catch (Exception ignored) {
                // Bucket can already exist in repeated E2E runs.
            }

            s3.putObject(
                    PutObjectRequest.builder().bucket(bucket).key(key).build(),
                    RequestBody.fromString(content, StandardCharsets.UTF_8)
            );
            ResponseBytes<?> response = s3.getObjectAsBytes(GetObjectRequest.builder().bucket(bucket).key(key).build());
            String fetched = response.asString(StandardCharsets.UTF_8);

            Map<String, Object> result = new LinkedHashMap<>();
            result.put("bucket", bucket);
            result.put("key", key);
            result.put("content", fetched);
            return result;
        }
    }

    private static Map<String, Object> writeCloudWatchLog(String marker) {
        String logGroup = "/lambda/image-function";
        String logStream = "image-stream-" + Instant.now().toEpochMilli();
        long now = Instant.now().toEpochMilli();

        try (CloudWatchLogsClient logs = CloudWatchLogsClient.builder().region(resolveRegion()).build()) {
            try {
                logs.createLogGroup(req -> req.logGroupName(logGroup));
            } catch (Exception ignored) {
                // Log group may already exist.
            }
            try {
                logs.createLogStream(req -> req.logGroupName(logGroup).logStreamName(logStream));
            } catch (Exception ignored) {
                // Log stream may already exist.
            }
            logs.putLogEvents(
                    PutLogEventsRequest.builder()
                            .logGroupName(logGroup)
                            .logStreamName(logStream)
                            .logEvents(
                                    InputLogEvent.builder()
                                            .timestamp(now)
                                            .message("[INFO] " + marker)
                                            .build()
                            )
                            .build()
            );
        }

        Map<String, Object> result = new LinkedHashMap<>();
        result.put("marker", marker);
        result.put("log_group", logGroup);
        result.put("log_stream", logStream);
        return result;
    }

    private static void emitMarker(String marker) {
        try {
            Map<String, Object> log = new LinkedHashMap<>();
            log.put("level", "INFO");
            log.put("message", marker);
            log.put("function", "lambda-image");
            System.out.println(MAPPER.writeValueAsString(log));
        } catch (Exception ignored) {
            System.out.println(marker);
        }
    }

    private static String readString(Map<String, Object> payload, String key, String fallback) {
        if (payload == null) {
            return fallback;
        }
        Object value = payload.get(key);
        if (value == null) {
            return fallback;
        }
        String text = String.valueOf(value).trim();
        return text.isEmpty() ? fallback : text;
    }

    private static Region resolveRegion() {
        String fromDefault = System.getenv("AWS_DEFAULT_REGION");
        if (fromDefault != null && !fromDefault.isBlank()) {
            return Region.of(fromDefault.trim());
        }
        String fromEnv = System.getenv("AWS_REGION");
        if (fromEnv != null && !fromEnv.isBlank()) {
            return Region.of(fromEnv.trim());
        }
        return Region.of(DEFAULT_REGION);
    }

    private static Map<String, Object> parseJsonMap(String raw) {
        if (raw == null || raw.isBlank()) {
            return new LinkedHashMap<>();
        }
        try {
            Map<String, Object> parsed = MAPPER.readValue(raw, MAP_TYPE);
            if (parsed == null) {
                return new LinkedHashMap<>();
            }
            return new LinkedHashMap<>(parsed);
        } catch (Exception ignored) {
            return new LinkedHashMap<>();
        }
    }

    private static Map<String, Object> castMap(Map<?, ?> source) {
        Map<String, Object> result = new LinkedHashMap<>();
        for (Map.Entry<?, ?> entry : source.entrySet()) {
            if (entry.getKey() == null) {
                continue;
            }
            result.put(String.valueOf(entry.getKey()), entry.getValue());
        }
        return result;
    }
}
