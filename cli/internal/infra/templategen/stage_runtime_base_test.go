// Where: cli/internal/infra/templategen/stage_runtime_base_test.go
// What: Tests for runtime-base staging filters.
// Why: Ensure generated artifacts do not include test/cache-only files.
package templategen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStageRuntimeBaseContextSkipsTestsAndPycache(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)

	writeTestFile(
		t,
		filepath.Join(root, "runtime-hooks", "python", "tests", "test_sitecustomize.py"),
		"def test_dummy():\n    assert True\n",
	)
	writeTestFile(
		t,
		filepath.Join(root, "runtime-hooks", "python", "sitecustomize", "site-packages", "__pycache__", "sitecustomize.cpython-313.pyc"),
		"bytecode",
	)
	writeTestFile(
		t,
		filepath.Join(root, "runtime-hooks", "python", "stray.pyc"),
		"bytecode",
	)

	ctx := stageContext{
		ProjectRoot: root,
		OutputDir:   filepath.Join(root, "out"),
	}
	if err := stageRuntimeBaseContext(ctx); err != nil {
		t.Fatalf("stage runtime base context: %v", err)
	}

	included := filepath.Join(root, "out", runtimeBaseContextDirName, "runtime-hooks", "python", "sitecustomize", "site-packages", "sitecustomize.py")
	if _, err := os.Stat(included); err != nil {
		t.Fatalf("expected staged runtime file: %v", err)
	}

	skippedTests := filepath.Join(root, "out", runtimeBaseContextDirName, "runtime-hooks", "python", "tests")
	if _, err := os.Stat(skippedTests); !os.IsNotExist(err) {
		if err != nil {
			t.Fatalf("stat skipped tests dir: %v", err)
		}
		t.Fatalf("did not expect tests dir in staged runtime base")
	}

	skippedPycache := filepath.Join(root, "out", runtimeBaseContextDirName, "runtime-hooks", "python", "sitecustomize", "site-packages", "__pycache__")
	if _, err := os.Stat(skippedPycache); !os.IsNotExist(err) {
		if err != nil {
			t.Fatalf("stat skipped __pycache__ dir: %v", err)
		}
		t.Fatalf("did not expect __pycache__ dir in staged runtime base")
	}

	skippedPyc := filepath.Join(root, "out", runtimeBaseContextDirName, "runtime-hooks", "python", "stray.pyc")
	if _, err := os.Stat(skippedPyc); !os.IsNotExist(err) {
		if err != nil {
			t.Fatalf("stat skipped .pyc file: %v", err)
		}
		t.Fatalf("did not expect .pyc file in staged runtime base")
	}
}
