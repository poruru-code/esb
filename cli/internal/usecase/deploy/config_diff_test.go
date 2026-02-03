// Where: cli/internal/usecase/deploy/config_diff_test.go
// What: Tests for config diff snapshot logic.
// Why: Validate merge summary counts across config updates.
package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDiffSnapshots(t *testing.T) {
	dir := t.TempDir()

	writeFile := func(name, contents string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	writeFile(
		"functions.yml",
		`functions:
  alpha:
    memory_size: 128
  beta:
    memory_size: 256
`,
	)
	writeFile(
		"routing.yml",
		`routes:
  - path: "/api/hello"
    method: "POST"
    function: "alpha"
`,
	)
	writeFile(
		"resources.yml",
		`resources:
  dynamodb:
    - TableName: table-a
      BillingMode: PAY_PER_REQUEST
  s3:
    - BucketName: bucket-a
`,
	)

	before, err := loadConfigSnapshot(dir)
	if err != nil {
		t.Fatalf("load before snapshot: %v", err)
	}

	writeFile(
		"functions.yml",
		`functions:
  alpha:
    memory_size: 256
  gamma:
    memory_size: 128
`,
	)
	writeFile(
		"routing.yml",
		`routes:
  - path: "/api/hello"
    method: "POST"
    function: "gamma"
  - path: "/api/echo"
    method: "POST"
    function: "gamma"
`,
	)
	writeFile(
		"resources.yml",
		`resources:
  dynamodb:
    - TableName: table-a
      BillingMode: PROVISIONED
  s3:
    - BucketName: bucket-b
`,
	)

	after, err := loadConfigSnapshot(dir)
	if err != nil {
		t.Fatalf("load after snapshot: %v", err)
	}

	diff := diffConfigSnapshots(before, after)
	if diff.Functions.Added != 1 || diff.Functions.Updated != 1 || diff.Functions.Removed != 1 || diff.Functions.Total != 2 {
		t.Fatalf("unexpected functions diff: %+v", diff.Functions)
	}
	if diff.Routes.Added != 1 || diff.Routes.Updated != 1 || diff.Routes.Removed != 0 || diff.Routes.Total != 2 {
		t.Fatalf("unexpected routes diff: %+v", diff.Routes)
	}
	if got := diff.Resources["dynamodb"]; got.Added != 0 || got.Updated != 1 || got.Removed != 0 || got.Total != 1 {
		t.Fatalf("unexpected dynamodb diff: %+v", got)
	}
	if got := diff.Resources["s3"]; got.Added != 1 || got.Updated != 0 || got.Removed != 1 || got.Total != 1 {
		t.Fatalf("unexpected s3 diff: %+v", got)
	}
}
