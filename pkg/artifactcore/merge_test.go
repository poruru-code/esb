package artifactcore

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestMergeRuntimeConfig_UsesManifestOrderAndLastWriteWins(t *testing.T) {
	root := t.TempDir()
	manifestDir := filepath.Join(root, "manifest")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatal(err)
	}

	aRoot := filepath.Join(root, "a")
	bRoot := filepath.Join(root, "b")
	writeYAMLFile(t, filepath.Join(aRoot, "config", "functions.yml"), map[string]any{
		"functions": map[string]any{
			"hello": map[string]any{"handler": "a.handler"},
		},
		"defaults": map[string]any{
			"environment": map[string]any{"LOG_LEVEL": "INFO"},
		},
	})
	writeYAMLFile(t, filepath.Join(aRoot, "config", "routing.yml"), map[string]any{
		"routes": []any{map[string]any{"path": "/hello", "method": "GET", "function": "hello"}},
	})
	writeYAMLFile(t, filepath.Join(aRoot, "config", "resources.yml"), map[string]any{
		"resources": map[string]any{"s3": []any{map[string]any{"BucketName": "bucket-a"}}},
	})

	writeYAMLFile(t, filepath.Join(bRoot, "config", "functions.yml"), map[string]any{
		"functions": map[string]any{
			"hello": map[string]any{"handler": "b.handler"},
			"bye":   map[string]any{"handler": "bye.handler"},
		},
		"defaults": map[string]any{
			"environment": map[string]any{"LOG_LEVEL": "DEBUG", "TRACE": "1"},
		},
	})
	writeYAMLFile(t, filepath.Join(bRoot, "config", "routing.yml"), map[string]any{
		"routes": []any{map[string]any{"path": "/hello", "method": "GET", "function": "bye"}},
	})

	manifest := ArtifactManifest{
		SchemaVersion: ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ArtifactRoot:     "../a",
				RuntimeConfigDir: "config",
				SourceTemplate:   ArtifactSourceTemplate{Path: "/tmp/a.yaml", SHA256: "sha-a"},
			},
			{
				ArtifactRoot:     "../b",
				RuntimeConfigDir: "config",
				SourceTemplate:   ArtifactSourceTemplate{Path: "/tmp/b.yaml", SHA256: "sha-b"},
			},
		},
	}
	for i := range manifest.Artifacts {
		e := manifest.Artifacts[i]
		e.ID = ComputeArtifactID(e.SourceTemplate.Path, e.SourceTemplate.Parameters, e.SourceTemplate.SHA256)
		manifest.Artifacts[i] = e
	}
	manifestPath := filepath.Join(manifestDir, "artifact.yml")
	if err := WriteArtifactManifest(manifestPath, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	outDir := filepath.Join(root, "out")
	if err := MergeRuntimeConfig(MergeRequest{ArtifactPath: manifestPath, OutputDir: outDir}); err != nil {
		t.Fatalf("MergeRuntimeConfig() error = %v", err)
	}

	functions := readYAMLFile(t, filepath.Join(outDir, "functions.yml"))
	fns := asMap(functions["functions"])
	hello := asMap(fns["hello"])
	if hello["handler"] != "b.handler" {
		t.Fatalf("expected hello handler b.handler, got %#v", hello["handler"])
	}
	defaults := asMap(functions["defaults"])
	env := asMap(defaults["environment"])
	if env["LOG_LEVEL"] != "INFO" {
		t.Fatalf("existing default should win: %#v", env["LOG_LEVEL"])
	}
	if env["TRACE"] != "1" {
		t.Fatalf("missing default key should be backfilled: %#v", env["TRACE"])
	}

	routing := readYAMLFile(t, filepath.Join(outDir, "routing.yml"))
	routes := asSlice(routing["routes"])
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	route := asMap(routes[0])
	if route["function"] != "bye" {
		t.Fatalf("expected route to be overwritten by b, got %#v", route["function"])
	}
}

func TestApply_ValidatesRequiredSecretEnv(t *testing.T) {
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "a")
	writeYAMLFile(t, filepath.Join(artifactRoot, "config", "functions.yml"), map[string]any{"functions": map[string]any{}})
	writeYAMLFile(t, filepath.Join(artifactRoot, "config", "routing.yml"), map[string]any{"routes": []any{}})

	manifest := ArtifactManifest{
		SchemaVersion: ArtifactSchemaVersionV1,
		Project:       "esb-dev",
		Env:           "dev",
		Mode:          "docker",
		Artifacts: []ArtifactEntry{
			{
				ArtifactRoot:      "../a",
				RuntimeConfigDir:  "config",
				RequiredSecretEnv: []string{"X_API_KEY", "AUTH_PASS"},
				SourceTemplate:    ArtifactSourceTemplate{Path: "/tmp/a.yaml", SHA256: "sha-a"},
			},
		},
	}
	manifest.Artifacts[0].ID = ComputeArtifactID("/tmp/a.yaml", nil, "sha-a")
	manifestPath := filepath.Join(root, "manifest", "artifact.yml")
	if err := WriteArtifactManifest(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(root, "out")
	err := Apply(ApplyRequest{ArtifactPath: manifestPath, OutputDir: outDir})
	if err == nil {
		t.Fatal("expected error for missing --secret-env")
	}

	secretEnv := filepath.Join(root, "secrets.env")
	if err := os.WriteFile(secretEnv, []byte("X_API_KEY=abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = Apply(ApplyRequest{ArtifactPath: manifestPath, OutputDir: outDir, SecretEnvPath: secretEnv})
	if err == nil {
		t.Fatal("expected error for missing AUTH_PASS")
	}

	if err := os.WriteFile(secretEnv, []byte("X_API_KEY=abc\nAUTH_PASS=pass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Apply(ApplyRequest{ArtifactPath: manifestPath, OutputDir: outDir, SecretEnvPath: secretEnv}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
}

func TestWithOutputDirLockSerializesConcurrentCalls(t *testing.T) {
	outputDir := t.TempDir()
	origWait := mergeLockWaitTimeout
	origPoll := mergeLockPollInterval
	mergeLockWaitTimeout = 3 * time.Second
	mergeLockPollInterval = 10 * time.Millisecond
	t.Cleanup(func() {
		mergeLockWaitTimeout = origWait
		mergeLockPollInterval = origPoll
	})

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstErrCh := make(chan error, 1)
	go func() {
		firstErrCh <- withOutputDirLock(outputDir, func() error {
			close(firstStarted)
			<-releaseFirst
			return nil
		})
	}()
	<-firstStarted

	secondErrCh := make(chan error, 1)
	secondDone := make(chan struct{})
	start := time.Now()
	go func() {
		secondErrCh <- withOutputDirLock(outputDir, func() error { return nil })
		close(secondDone)
	}()

	time.Sleep(80 * time.Millisecond)
	select {
	case <-secondDone:
		t.Fatal("second lock call finished before first released")
	default:
	}

	close(releaseFirst)

	if err := <-firstErrCh; err != nil {
		t.Fatalf("first lock call error: %v", err)
	}
	if err := <-secondErrCh; err != nil {
		t.Fatalf("second lock call error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 70*time.Millisecond {
		t.Fatalf("expected second call to wait for lock, elapsed=%s", elapsed)
	}
}

func TestWithOutputDirLockTimesOutWhenLockIsHeld(t *testing.T) {
	outputDir := t.TempDir()
	lockPath := filepath.Join(outputDir, mergeLockFileName)
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatalf("create lock file: %v", err)
	}

	origWait := mergeLockWaitTimeout
	origPoll := mergeLockPollInterval
	mergeLockWaitTimeout = 40 * time.Millisecond
	mergeLockPollInterval = 10 * time.Millisecond
	t.Cleanup(func() {
		mergeLockWaitTimeout = origWait
		mergeLockPollInterval = origPoll
	})

	err := withOutputDirLock(outputDir, func() error { return nil })
	if err == nil {
		t.Fatal("expected timeout error when lock is held")
	}
}

func TestWithOutputDirLockRecoversStaleLockFile(t *testing.T) {
	outputDir := t.TempDir()
	lockPath := filepath.Join(outputDir, mergeLockFileName)
	if err := os.WriteFile(lockPath, []byte("999999\n"), 0o600); err != nil {
		t.Fatalf("create stale lock file: %v", err)
	}

	if err := withOutputDirLock(outputDir, func() error { return nil }); err != nil {
		t.Fatalf("withOutputDirLock should recover stale lock: %v", err)
	}
}

func TestWithOutputDirLockIgnoresUnparseableLockOwner(t *testing.T) {
	outputDir := t.TempDir()
	lockPath := filepath.Join(outputDir, mergeLockFileName)
	if err := os.WriteFile(lockPath, []byte("not-a-pid\n"), 0o600); err != nil {
		t.Fatalf("create lock file: %v", err)
	}

	origWait := mergeLockWaitTimeout
	origPoll := mergeLockPollInterval
	mergeLockWaitTimeout = 40 * time.Millisecond
	mergeLockPollInterval = 10 * time.Millisecond
	t.Cleanup(func() {
		mergeLockWaitTimeout = origWait
		mergeLockPollInterval = origPoll
	})

	err := withOutputDirLock(outputDir, func() error { return nil })
	if err == nil {
		t.Fatal("expected timeout error for unparseable lock owner")
	}
}

func TestReadLockOwnerPIDParsesLegacyPrefix(t *testing.T) {
	outputDir := t.TempDir()
	lockPath := filepath.Join(outputDir, mergeLockFileName)
	if err := os.WriteFile(lockPath, []byte("pid=1234\n"), 0o600); err != nil {
		t.Fatalf("write lock file: %v", err)
	}

	pid, ok, err := readLockOwnerPID(lockPath)
	if err != nil {
		t.Fatalf("readLockOwnerPID() error = %v", err)
	}
	if !ok {
		t.Fatal("expected lock owner PID")
	}
	if pid != 1234 {
		t.Fatalf("pid = %d, want 1234", pid)
	}
}

func TestReadLockOwnerPIDRejectsInvalidValues(t *testing.T) {
	outputDir := t.TempDir()
	lockPath := filepath.Join(outputDir, mergeLockFileName)

	if err := os.WriteFile(lockPath, []byte("\n"), 0o600); err != nil {
		t.Fatalf("write lock file: %v", err)
	}
	if pid, ok, err := readLockOwnerPID(lockPath); err != nil || ok || pid != 0 {
		t.Fatalf("blank line should be ignored: pid=%d ok=%v err=%v", pid, ok, err)
	}

	if err := os.WriteFile(lockPath, []byte("pid=-10\n"), 0o600); err != nil {
		t.Fatalf("write lock file: %v", err)
	}
	if pid, ok, err := readLockOwnerPID(lockPath); err != nil || ok || pid != 0 {
		t.Fatalf("negative pid should be ignored: pid=%d ok=%v err=%v", pid, ok, err)
	}

	if err := os.Remove(lockPath); err != nil {
		t.Fatalf("remove lock file: %v", err)
	}
	if pid, ok, err := readLockOwnerPID(lockPath); err != nil || ok || pid != 0 {
		t.Fatalf("missing file should be treated as no owner: pid=%d ok=%v err=%v", pid, ok, err)
	}
}

func TestIsProcessAliveForCurrentAndMissingPID(t *testing.T) {
	alive, err := isProcessAlive(os.Getpid())
	if err != nil {
		t.Fatalf("isProcessAlive(current pid) error = %v", err)
	}
	if !alive {
		t.Fatal("current process should be alive")
	}

	alive, err = isProcessAlive(99999999)
	if err != nil {
		t.Fatalf("isProcessAlive(missing pid) error = %v", err)
	}
	if alive {
		t.Fatal("expected missing PID to be treated as dead")
	}
}

func TestReadLockOwnerPIDReturnsErrorWhenPathIsDirectory(t *testing.T) {
	outputDir := t.TempDir()
	lockDir := filepath.Join(outputDir, "lockdir")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("mkdir lockdir: %v", err)
	}

	_, _, err := readLockOwnerPID(lockDir)
	if err == nil {
		t.Fatal("expected read error")
	}
	if !strings.Contains(err.Error(), "read merge lock file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeYAMLFile(t *testing.T, path string, value map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readYAMLFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAtomicWriteYAMLUsesTwoSpaceIndent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resources.yml")
	payload := map[string]any{
		"resources": map[string]any{
			"s3": []any{
				map[string]any{"BucketName": "bucket-a"},
			},
		},
	}

	if err := atomicWriteYAML(path, payload); err != nil {
		t.Fatalf("atomicWriteYAML: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output yaml: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "\n    s3:") {
		t.Fatalf("expected 2-space indentation for map entries, got: %s", content)
	}
	if !strings.Contains(content, "\n  s3:") {
		t.Fatalf("expected s3 entry with 2-space indentation, got: %s", content)
	}
}
