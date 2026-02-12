// Where: cli/internal/usecase/deploy/config_diff_test.go
// What: Tests for config diff snapshot logic.
// Why: Validate merge summary counts across config updates.
package deploy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domaincfg "github.com/poruru/edge-serverless-box/cli/internal/domain/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
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

func TestEmitConfigMergeSummaryRendersRows(t *testing.T) {
	capture := &captureUI{}
	diff := domaincfg.Diff{
		Functions: domaincfg.Counts{Added: 1, Updated: 2, Removed: 3, Total: 4},
		Routes:    domaincfg.Counts{Added: 5, Updated: 6, Removed: 7, Total: 8},
		Resources: map[string]domaincfg.Counts{
			"dynamodb": {Added: 1, Updated: 0, Removed: 0, Total: 1},
			"s3":       {Added: 0, Updated: 0, Removed: 0, Total: 0},
			"layers":   {Added: 0, Updated: 1, Removed: 0, Total: 1},
		},
	}

	emitConfigMergeSummary(capture, "/tmp/config", diff)
	if capture.blockCalls != 1 {
		t.Fatalf("expected one block call, got %d", capture.blockCalls)
	}
	if capture.blockEmoji != "🧩" || capture.blockTitle != "Config merge summary" {
		t.Fatalf("unexpected block metadata: emoji=%q title=%q", capture.blockEmoji, capture.blockTitle)
	}
	if !hasRow(capture.blockRows, "Staging config") {
		t.Fatalf("expected staging config row, got %#v", capture.blockRows)
	}
	if !hasRow(capture.blockRows, "Resources.dynamodb") {
		t.Fatalf("expected dynamodb row, got %#v", capture.blockRows)
	}
	if hasRow(capture.blockRows, "Resources.s3") {
		t.Fatalf("did not expect zero-count s3 row, got %#v", capture.blockRows)
	}
	if !hasRow(capture.blockRows, "Resources.layers") {
		t.Fatalf("expected layers row, got %#v", capture.blockRows)
	}
}

func TestEmitTemplateDeltaSummaryRendersRows(t *testing.T) {
	capture := &captureUI{}
	diff := domaincfg.Diff{
		Functions: domaincfg.Counts{Added: 1, Updated: 1, Removed: 0, Total: 2},
		Routes:    domaincfg.Counts{Added: 0, Updated: 1, Removed: 0, Total: 1},
		Resources: map[string]domaincfg.Counts{
			"dynamodb": {Added: 0, Updated: 0, Removed: 0, Total: 0},
			"s3":       {Added: 1, Updated: 0, Removed: 0, Total: 1},
			"layers":   {Added: 0, Updated: 0, Removed: 0, Total: 0},
		},
	}

	emitTemplateDeltaSummary(capture, "/tmp/template-config", diff)
	if capture.blockCalls != 1 {
		t.Fatalf("expected one block call, got %d", capture.blockCalls)
	}
	if capture.blockEmoji != "🧾" || capture.blockTitle != "Template delta summary" {
		t.Fatalf("unexpected block metadata: emoji=%q title=%q", capture.blockEmoji, capture.blockTitle)
	}
	if !hasRow(capture.blockRows, "Template config") {
		t.Fatalf("expected template config row, got %#v", capture.blockRows)
	}
	if !hasRow(capture.blockRows, "Resources.s3") {
		t.Fatalf("expected s3 row, got %#v", capture.blockRows)
	}
	if hasRow(capture.blockRows, "Resources.dynamodb") {
		t.Fatalf("did not expect zero-count dynamodb row, got %#v", capture.blockRows)
	}
}

func TestResolveTemplateConfigDirRequiresTemplatePath(t *testing.T) {
	_, err := resolveTemplateConfigDir(" ", "", "dev")
	if !errors.Is(err, errTemplatePathRequired) {
		t.Fatalf("expected errTemplatePathRequired, got %v", err)
	}
}

func TestResolveTemplateConfigDirResolvesRelativeOutputDir(t *testing.T) {
	templatePath := filepath.Join("/tmp", "service", "template.yaml")
	got, err := resolveTemplateConfigDir(templatePath, "./out", "dev")
	if err != nil {
		t.Fatalf("resolve template config dir: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join("service", "out", "dev", "config")) {
		t.Fatalf("unexpected config dir: %q", got)
	}
}

func TestLoadConfigSnapshotDoesNotErrorWhenFilesMissing(t *testing.T) {
	dir := t.TempDir()
	snapshot, err := loadConfigSnapshot(dir)
	if err != nil {
		t.Fatalf("load config snapshot: %v", err)
	}
	if len(snapshot.Functions) != 0 {
		t.Fatalf("expected no functions, got %#v", snapshot.Functions)
	}
	if len(snapshot.Routes) != 0 {
		t.Fatalf("expected no routes, got %#v", snapshot.Routes)
	}
	if len(snapshot.Resources) != 0 {
		t.Fatalf("expected no resources, got %#v", snapshot.Resources)
	}
}

type captureUI struct {
	blockCalls int
	blockEmoji string
	blockTitle string
	blockRows  []ui.KeyValue
}

func (c *captureUI) Info(string) {}

func (c *captureUI) Warn(string) {}

func (c *captureUI) Success(string) {}

func (c *captureUI) Block(emoji, title string, rows []ui.KeyValue) {
	c.blockCalls++
	c.blockEmoji = emoji
	c.blockTitle = title
	c.blockRows = append([]ui.KeyValue(nil), rows...)
}

func hasRow(rows []ui.KeyValue, key string) bool {
	for _, row := range rows {
		if row.Key == key {
			return true
		}
	}
	return false
}
