package provisioner

import (
	"context"
	"io"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/generator/schema"
)

func TestProvisionS3LifecycleMapping(t *testing.T) {
	fake := &fakeS3{}
	buckets := []generator.S3Spec{
		{
			BucketName: "lifecycle-bucket",
			LifecycleConfiguration: &schema.AWSS3BucketLifecycleConfiguration{
				Rules: []schema.AWSS3BucketRule{
					{
						Id:               "Rule1",
						Status:           "Enabled",
						Prefix:           "logs/",
						ExpirationInDays: 30,
					},
					{
						Id:               "Rule2",
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

	config, ok := val.(*schema.AWSS3BucketLifecycleConfiguration)
	if !ok {
		t.Fatalf("expected *schema.AWSS3BucketLifecycleConfiguration, got %T", val)
	}

	if len(config.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(config.Rules))
	}

	// Verify first rule
	r1 := config.Rules[0]
	if r1.Id != "Rule1" || r1.Status != "Enabled" || r1.Prefix != "logs/" || r1.ExpirationInDays != 30 {
		t.Errorf("unexpected Rule1: %+v", r1)
	}

	// Verify second rule
	r2 := config.Rules[1]
	if r2.Id != "Rule2" || r2.Status != "Disabled" || r2.Prefix != nil || r2.ExpirationInDays != 7 {
		t.Errorf("unexpected Rule2: %+v", r2)
	}
}
