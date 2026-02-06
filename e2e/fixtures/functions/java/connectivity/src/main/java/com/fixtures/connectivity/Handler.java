// Where: e2e/fixtures/functions/java/connectivity/src/main/java/com/fixtures/connectivity/Handler.java
// What: Java connectivity Lambda fixture for E2E smoke coverage.
// Why: Exercise javaagent endpoint overrides and CloudWatch Logs passthrough.
package com.fixtures.connectivity;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.Base64;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import software.amazon.awssdk.core.SdkBytes;
import software.amazon.awssdk.regions.Region;
import software.amazon.awssdk.services.cloudwatchlogs.CloudWatchLogsClient;
import software.amazon.awssdk.services.cloudwatchlogs.model.InputLogEvent;
import software.amazon.awssdk.services.cloudwatchlogs.model.PutLogEventsRequest;
import software.amazon.awssdk.services.dynamodb.DynamoDbClient;
import software.amazon.awssdk.services.dynamodb.model.AttributeValue;
import software.amazon.awssdk.services.dynamodb.model.PutItemRequest;
import software.amazon.awssdk.services.lambda.LambdaClient;
import software.amazon.awssdk.services.lambda.model.InvokeRequest;
import software.amazon.awssdk.services.lambda.model.InvokeResponse;
import software.amazon.awssdk.services.s3.S3Client;
import software.amazon.awssdk.services.s3.model.CreateBucketRequest;
import software.amazon.awssdk.services.s3.model.PutObjectRequest;
import software.amazon.awssdk.core.sync.RequestBody;

public final class Handler {
    private static final ObjectMapper MAPPER = new ObjectMapper().findAndRegisterModules();
    private static final TypeReference<Map<String, Object>> MAP_TYPE = new TypeReference<>() {};

    private static final String DEFAULT_TABLE = "e2e-test-table";
    private static final String DEFAULT_BUCKET = "e2e-test-bucket";
    private static final String DEFAULT_LAMBDA_TARGET = "lambda-echo";

    public Map<String, Object> handleRequest(Map<String, Object> event) {
        if (event != null && Boolean.TRUE.equals(event.get("ping"))) {
            return createResponse(200, Map.of("message", "pong"));
        }

        Map<String, Object> body = parseBody(event);
        String action = getString(body, "action", "hello");

        try {
            return switch (action) {
                case "echo" -> handleEcho(body);
                case "dynamodb_put" -> handleDynamo(body);
                case "s3_put" -> handleS3(body);
                case "lambda_invoke" -> handleLambdaInvoke(body);
                case "chain_invoke" -> handleChainInvoke(body);
                case "test_cloudwatch" -> handleCloudWatch(body);
                default -> createResponse(200, Map.of("success", true, "action", action));
            };
        } catch (Exception e) {
            return createResponse(
                    500,
                    Map.of(
                            "success", false,
                            "action", action,
                            "error", String.valueOf(e)
                    )
            );
        }
    }

    private Map<String, Object> handleEcho(Map<String, Object> body) {
        String message = getString(body, "message", "hello");
        return createResponse(
                200,
                Map.of(
                        "success", true,
                        "action", "echo",
                        "message", "Echo: " + message
                )
        );
    }

    private Map<String, Object> handleDynamo(Map<String, Object> body) {
        String table = getenvOrDefault("E2E_TABLE_NAME", DEFAULT_TABLE);
        String key = getString(body, "key", "java-connectivity");
        String value = getString(body, "value", "ok");

        try (DynamoDbClient dynamo = DynamoDbClient.builder().region(resolveRegion()).build()) {
            Map<String, AttributeValue> item = new LinkedHashMap<>();
            item.put("id", AttributeValue.builder().s(key).build());
            item.put("value", AttributeValue.builder().s(value).build());
            dynamo.putItem(PutItemRequest.builder().tableName(table).item(item).build());
        }

        return createResponse(200, Map.of("success", true, "action", "dynamodb_put"));
    }

    private Map<String, Object> handleS3(Map<String, Object> body) {
        String bucket = getString(body, "bucket", getenvOrDefault("E2E_BUCKET", DEFAULT_BUCKET));
        String key = getString(body, "key", "java-connectivity.txt");
        String content = getString(body, "content", "ok");

        try (S3Client s3 = S3Client.builder().region(resolveRegion()).build()) {
            try {
                s3.createBucket(CreateBucketRequest.builder().bucket(bucket).build());
            } catch (Exception ignored) {
                // bucket may already exist
            }
            s3.putObject(
                    PutObjectRequest.builder().bucket(bucket).key(key).build(),
                    RequestBody.fromString(content)
            );
        }

        return createResponse(200, Map.of("success", true, "action", "s3_put"));
    }

    private Map<String, Object> handleLambdaInvoke(Map<String, Object> body) throws Exception {
        String target = getString(body, "target", DEFAULT_LAMBDA_TARGET);
        String message = getString(body, "message", "hello-from-java");
        return createResponse(200, invokeLambda(target, message, "lambda_invoke"));
    }

    private Map<String, Object> handleChainInvoke(Map<String, Object> body) throws Exception {
        String target = getString(body, "target", DEFAULT_LAMBDA_TARGET);
        String message = getString(body, "message", "from-java");
        return createResponse(200, invokeLambda(target, message, "chain_invoke"));
    }

    private Map<String, Object> invokeLambda(String target, String message, String action)
            throws Exception {
        Map<String, Object> payload = Map.of("message", message);
        String json = MAPPER.writeValueAsString(payload);

        InvokeRequest request = InvokeRequest.builder()
                .functionName(target)
                .payload(SdkBytes.fromUtf8String(json))
                .build();

        InvokeResponse response;
        try (LambdaClient lambda = LambdaClient.builder().region(resolveRegion()).build()) {
            response = lambda.invoke(request);
        }

        String bodyText = response.payload() == null ? "" : response.payload().asUtf8String();
        Map<String, Object> child = parseJsonMap(bodyText);

        Map<String, Object> result = new LinkedHashMap<>();
        result.put("success", true);
        result.put("action", action);
        result.put("child", child.isEmpty() ? bodyText : child);
        return result;
    }

    private Map<String, Object> handleCloudWatch(Map<String, Object> body) {
        String logGroup = "/aws/lambda/java-connectivity";
        String logStream = "test-stream-" + System.currentTimeMillis();

        long now = System.currentTimeMillis();
        List<InputLogEvent> events = List.of(
                InputLogEvent.builder().timestamp(now).message("[INFO] Java CloudWatch log test").build(),
                InputLogEvent.builder().timestamp(now + 1).message("[DEBUG] Debug log from java connectivity").build(),
                InputLogEvent.builder().timestamp(now + 2).message("[ERROR] Error log from java connectivity").build(),
                InputLogEvent.builder().timestamp(now + 3).message("CloudWatch Logs E2E verification successful!").build()
        );

        try (CloudWatchLogsClient logs = CloudWatchLogsClient.builder().region(resolveRegion()).build()) {
            try {
                logs.createLogGroup(builder -> builder.logGroupName(logGroup));
            } catch (Exception ignored) {
                // Already exists
            }
            try {
                logs.createLogStream(builder -> builder.logGroupName(logGroup).logStreamName(logStream));
            } catch (Exception ignored) {
                // Already exists
            }
            logs.putLogEvents(
                    PutLogEventsRequest.builder()
                            .logGroupName(logGroup)
                            .logStreamName(logStream)
                            .logEvents(events)
                            .build()
            );
        }

        return createResponse(
                200,
                Map.of(
                        "success", true,
                        "action", "test_cloudwatch",
                        "log_group", logGroup,
                        "log_stream", logStream
                )
        );
    }

    private Map<String, Object> parseBody(Map<String, Object> event) {
        if (event == null) {
            return Collections.emptyMap();
        }
        Object body = event.get("body");
        if (body instanceof Map<?, ?> bodyMap) {
            return castMap(bodyMap);
        }
        if (body instanceof String bodyText) {
            if (Boolean.TRUE.equals(event.get("isBase64Encoded"))) {
                bodyText = decodeBase64(bodyText);
            }
            Map<String, Object> parsed = parseJsonMap(bodyText);
            return parsed.isEmpty() ? Map.of("raw", bodyText) : parsed;
        }
        return event;
    }

    private Map<String, Object> parseJsonMap(String bodyText) {
        if (bodyText == null || bodyText.isBlank()) {
            return Collections.emptyMap();
        }
        try {
            Map<String, Object> parsed = MAPPER.readValue(bodyText, MAP_TYPE);
            return parsed == null ? Collections.emptyMap() : parsed;
        } catch (Exception ignored) {
            return Collections.emptyMap();
        }
    }

    private Map<String, Object> createResponse(int statusCode, Map<String, Object> body) {
        Map<String, Object> response = new LinkedHashMap<>();
        response.put("statusCode", statusCode);
        response.put("headers", Map.of("Content-Type", "application/json"));
        response.put("body", writeJson(body));
        return response;
    }

    private String writeJson(Map<String, Object> value) {
        try {
            return MAPPER.writeValueAsString(value);
        } catch (Exception ignored) {
            return "{}";
        }
    }

    private static Region resolveRegion() {
        String region = getenvOrDefault("AWS_DEFAULT_REGION", getenvOrDefault("AWS_REGION", "ap-northeast-1"));
        return Region.of(region);
    }

    private static String decodeBase64(String value) {
        try {
            byte[] decoded = Base64.getDecoder().decode(value);
            return new String(decoded, StandardCharsets.UTF_8);
        } catch (IllegalArgumentException ignored) {
            return value;
        }
    }

    private static String getString(Map<String, Object> map, String key, String fallback) {
        if (map == null) {
            return fallback;
        }
        Object value = map.get(key);
        if (value == null) {
            return fallback;
        }
        String str = value.toString().trim();
        return str.isEmpty() ? fallback : str;
    }

    private static Map<String, Object> castMap(Map<?, ?> raw) {
        Map<String, Object> result = new LinkedHashMap<>();
        for (Map.Entry<?, ?> entry : raw.entrySet()) {
            if (entry.getKey() == null) {
                continue;
            }
            result.put(entry.getKey().toString(), entry.getValue());
        }
        return result;
    }

    private static String getenvOrDefault(String key, String fallback) {
        String value = System.getenv(key);
        if (value == null || value.isBlank()) {
            return fallback;
        }
        return value.trim();
    }
}
