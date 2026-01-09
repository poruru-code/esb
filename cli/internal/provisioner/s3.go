// Where: cli/internal/provisioner/s3.go
// What: S3 provisioning helpers.
// Why: Create S3-compatible buckets based on SAM resources.
package provisioner

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/generator"
)

type S3API interface {
	ListBuckets(ctx context.Context) ([]string, error)
	CreateBucket(ctx context.Context, name string) error
}

func provisionS3(ctx context.Context, client S3API, buckets []generator.S3Spec, out io.Writer) {
	if client == nil || len(buckets) == 0 {
		return
	}
	if out == nil {
		out = io.Discard
	}

	existingBuckets := map[string]struct{}{}
	if names, err := client.ListBuckets(ctx); err == nil {
		for _, name := range names {
			existingBuckets[name] = struct{}{}
		}
	}

	for _, bucket := range buckets {
		name := strings.TrimSpace(bucket.BucketName)
		if name == "" {
			continue
		}
		if _, ok := existingBuckets[name]; ok {
			fmt.Fprintf(out, "Bucket '%s' already exists. Skipping.\n", name)
			continue
		}

		if err := client.CreateBucket(ctx, name); err != nil {
			fmt.Fprintf(out, "❌ Failed to create bucket %s: %v\n", name, err)
			continue
		}
		fmt.Fprintf(out, "✅ Created S3 Bucket: %s\n", name)
	}
}
