package provisioner

import (
	"context"
	"io"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
)

func TestProvisionS3LifecycleMapping(t *testing.T) {
	fake := &fakeS3{}
	buckets := []manifest.S3Spec{
		{
			BucketName: "lifecycle-bucket",
			LifecycleConfiguration: &manifest.S3LifecycleConfiguration{
				Rules: []manifest.S3LifecycleRule{
					{
						ID:               "Rule1",
						Status:           "Enabled",
						Prefix:           "logs/",
						ExpirationInDays: 30,
					},
					{
						ID:               "Rule2",
						Status:           "Disabled",
						ExpirationInDays: 7,
					},
				},
			},
		},
	}

	provisionS3(context.Background(), fake, buckets, io.Discard)

	val, ok := fake.lifecycleConf["lifecycle-bucket"]
	if !ok {
		t.Fatal("expected lifecycle configuration to be set")
	}

	config, ok := val.(*manifest.S3LifecycleConfiguration)
	if !ok {
		t.Fatalf("expected *manifest.S3LifecycleConfiguration, got %T", val)
	}

	if len(config.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(config.Rules))
	}

	// Verify first rule
	r1 := config.Rules[0]
	if r1.ID != "Rule1" || r1.Status != "Enabled" || r1.Prefix != "logs/" || r1.ExpirationInDays != 30 {
		t.Errorf("unexpected Rule1: %+v", r1)
	}

	// Verify second rule
	r2 := config.Rules[1]
	if r2.ID != "Rule2" || r2.Status != "Disabled" || r2.Prefix != nil || r2.ExpirationInDays != 7 {
		t.Errorf("unexpected Rule2: %+v", r2)
	}
}
