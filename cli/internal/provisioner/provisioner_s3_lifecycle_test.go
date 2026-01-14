package provisioner

import (
	"context"
	"io"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/generator"
)

func TestProvisionS3LifecycleMapping(t *testing.T) {
	fake := &fakeS3{}
	buckets := []generator.S3Spec{
		{
			BucketName: "lifecycle-bucket",
			LifecycleConfiguration: map[string]any{
				"Rules": []any{
					map[string]any{
						"Id":               "Rule1",
						"Status":           "Enabled",
						"Prefix":           "logs/",
						"ExpirationInDays": 30,
					},
					map[string]any{
						"Id":               "Rule2",
						"Status":           "Disabled",
						"ExpirationInDays": 7,
					},
				},
			},
		},
	}

	provisionS3(context.Background(), fake, buckets, io.Discard)

	rules, ok := fake.lifecycleConf["lifecycle-bucket"]
	if !ok {
		t.Fatal("expected lifecycle configuration to be set")
	}

	rulesMap, ok := rules.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", rules)
	}

	items, ok := rulesMap["Rules"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 rules, got %v", rulesMap["Rules"])
	}

	// Verify first rule
	r1 := items[0].(map[string]any)
	if r1["Id"] != "Rule1" || r1["Status"] != "Enabled" || r1["Prefix"] != "logs/" || r1["ExpirationInDays"] != 30 {
		t.Errorf("unexpected Rule1: %v", r1)
	}

	// Verify second rule
	r2 := items[1].(map[string]any)
	if r2["Id"] != "Rule2" || r2["Status"] != "Disabled" || r2["Prefix"] != nil || r2["ExpirationInDays"] != 7 {
		t.Errorf("unexpected Rule2: %v", r2)
	}
}

func TestProvisionS3LifecycleInvalidData(t *testing.T) {
	fake := &fakeS3{}
	// Test with invalid format that should be handled gracefully (i.e. not panic, though it might report error to io.Writer)
	buckets := []generator.S3Spec{
		{
			BucketName:             "bad-bucket",
			LifecycleConfiguration: "not-a-map",
		},
	}

	provisionS3(context.Background(), fake, buckets, io.Discard)

	if _, ok := fake.lifecycleConf["bad-bucket"]; ok {
		t.Error("expected no lifecycle configuration for invalid format")
	}
}
