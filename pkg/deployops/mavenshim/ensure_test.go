package mavenshim

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordRunner struct {
	mu       sync.Mutex
	commands [][]string
	hook     func(cmd []string) error
}

func (r *recordRunner) Run(cmd []string) error {
	r.mu.Lock()
	r.commands = append(r.commands, append([]string(nil), cmd...))
	r.mu.Unlock()
	if r.hook == nil {
		return nil
	}
	return r.hook(cmd)
}

func (r *recordRunner) snapshot() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]string, 0, len(r.commands))
	for _, cmd := range r.commands {
		out = append(out, append([]string(nil), cmd...))
	}
	return out
}

func assertCommandContains(t *testing.T, cmd []string, key, expected string) {
	t.Helper()
	for i := 0; i+1 < len(cmd); i++ {
		if cmd[i] == key && cmd[i+1] == expected {
			return
		}
	}
	t.Fatalf("expected command to contain %s %s, got: %v", key, expected, cmd)
}

func TestEnsureImageBuildsAndPushesWithHostRegistry(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://proxy.example:8080")
	t.Setenv("HTTPS_PROXY", "http://secure-proxy.example:8443")
	t.Setenv("NO_PROXY", "localhost,127.0.0.1,registry")

	runner := &recordRunner{}
	result, err := EnsureImage(EnsureInput{
		BaseImage:    "maven:3.9.11-eclipse-temurin-21",
		HostRegistry: "127.0.0.1:5010",
		Runner:       runner,
		ImageExists: func(string) bool {
			return false
		},
	})
	if err != nil {
		t.Fatalf("EnsureImage() error = %v", err)
	}
	expectedShim := "127.0.0.1:5010/" + deriveShimImageTag("maven:3.9.11-eclipse-temurin-21")
	if result.ShimImage != expectedShim {
		t.Fatalf("unexpected shim image: %s", result.ShimImage)
	}

	commands := runner.snapshot()
	if len(commands) != 2 {
		t.Fatalf("expected build + push commands, got: %v", commands)
	}
	buildCmd := commands[0]
	if len(buildCmd) < 4 || !slices.Equal(buildCmd[0:3], []string{"docker", "buildx", "build"}) {
		t.Fatalf("unexpected build command: %v", buildCmd)
	}
	assertCommandContains(t, buildCmd, "--build-arg", "BASE_MAVEN_IMAGE=maven:3.9.11-eclipse-temurin-21")
	assertCommandContains(t, buildCmd, "--build-arg", "HTTP_PROXY=http://proxy.example:8080")
	assertCommandContains(t, buildCmd, "--build-arg", "http_proxy=http://proxy.example:8080")
	assertCommandContains(t, buildCmd, "--build-arg", "HTTPS_PROXY=http://secure-proxy.example:8443")
	assertCommandContains(t, buildCmd, "--build-arg", "https_proxy=http://secure-proxy.example:8443")
	assertCommandContains(t, buildCmd, "--build-arg", "NO_PROXY=localhost,127.0.0.1,registry")
	assertCommandContains(t, buildCmd, "--build-arg", "no_proxy=localhost,127.0.0.1,registry")
	if !slices.Equal(commands[1], []string{"docker", "push", result.ShimImage}) {
		t.Fatalf("unexpected push command: %v", commands[1])
	}
}

func TestEnsureImageSkipsBuildWhenImageAlreadyExists(t *testing.T) {
	runner := &recordRunner{}
	result, err := EnsureImage(EnsureInput{
		BaseImage: "maven:3.9.11-eclipse-temurin-21",
		Runner:    runner,
		ImageExists: func(string) bool {
			return true
		},
	})
	if err != nil {
		t.Fatalf("EnsureImage() error = %v", err)
	}
	expectedShim := deriveShimImageTag("maven:3.9.11-eclipse-temurin-21")
	if result.ShimImage != expectedShim {
		t.Fatalf("unexpected shim image: %s", result.ShimImage)
	}
	if got := len(runner.snapshot()); got != 0 {
		t.Fatalf("expected no commands, got: %v", runner.snapshot())
	}
}

func TestEnsureImageNoCacheForcesBuild(t *testing.T) {
	runner := &recordRunner{}
	_, err := EnsureImage(EnsureInput{
		BaseImage: "maven:3.9.11-eclipse-temurin-21",
		NoCache:   true,
		Runner:    runner,
		ImageExists: func(string) bool {
			return true
		},
	})
	if err != nil {
		t.Fatalf("EnsureImage() error = %v", err)
	}
	commands := runner.snapshot()
	if len(commands) != 1 {
		t.Fatalf("expected one build command, got: %v", commands)
	}
	if !slices.Contains(commands[0], "--no-cache") {
		t.Fatalf("expected --no-cache in build command: %v", commands[0])
	}
}

func TestEnsureImageRejectsInvalidProxyConfig(t *testing.T) {
	t.Setenv("HTTP_PROXY", "://invalid")
	runner := &recordRunner{}
	_, err := EnsureImage(EnsureInput{
		BaseImage: "maven:3.9.11-eclipse-temurin-21",
		NoCache:   true,
		Runner:    runner,
		ImageExists: func(string) bool {
			return true
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid proxy configuration for maven shim build") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAcquireShimLockTimesOutWhenAlreadyHeld(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "maven-shim.lock")
	release, err := acquireShimLock(lockPath, time.Second, 10*time.Millisecond, time.Hour)
	if err != nil {
		t.Fatalf("acquireShimLock() first acquire error = %v", err)
	}
	defer release()

	_, err = acquireShimLock(lockPath, 120*time.Millisecond, 20*time.Millisecond, time.Hour)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout acquiring maven shim lock") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEvictStaleShimLockRemovesExpiredFile(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "maven-shim.lock")
	if err := os.WriteFile(lockPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write lock file: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatalf("set lock mtime: %v", err)
	}
	evicted, err := evictStaleShimLock(lockPath, time.Minute)
	if err != nil {
		t.Fatalf("evictStaleShimLock() error = %v", err)
	}
	if !evicted {
		t.Fatal("expected stale lock eviction")
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("expected lock to be removed, stat err: %v", err)
	}
}

func TestAcquireShimLockAllowsNextContenderAfterRelease(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "maven-shim.lock")
	releaseFirst, err := acquireShimLock(lockPath, time.Second, 10*time.Millisecond, time.Hour)
	if err != nil {
		t.Fatalf("acquireShimLock() first acquire error = %v", err)
	}

	type lockResult struct {
		release func()
		err     error
	}
	resultCh := make(chan lockResult, 1)
	go func() {
		release, err := acquireShimLock(lockPath, time.Second, 10*time.Millisecond, time.Hour)
		resultCh <- lockResult{release: release, err: err}
	}()

	time.Sleep(80 * time.Millisecond)
	releaseFirst()

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("second acquire failed: %v", result.err)
		}
		if result.release == nil {
			t.Fatal("second acquire returned nil release function")
		}
		result.release()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second acquire")
	}
}

func TestMavenWrapperAcceptsCaseInsensitiveProxySchemes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script execution is not portable to windows")
	}
	contextDir, cleanup, err := materializeBuildContext()
	if err != nil {
		t.Fatalf("materializeBuildContext() error = %v", err)
	}
	defer cleanup()

	fakeBinDir := t.TempDir()
	fakeMaven := filepath.Join(fakeBinDir, "fake-mvn")
	captureFile := filepath.Join(fakeBinDir, "captured")
	fakeMavenScript := strings.Join([]string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"args=(\"$@\")",
		"settings=\"\"",
		"for ((i=0; i<${#args[@]}; i++)); do",
		"  if [[ \"${args[$i]}\" == \"-s\" ]]; then",
		"    next=$((i+1))",
		"    settings=\"${args[$next]:-}\"",
		"    break",
		"  fi",
		"done",
		"if [[ -z \"$settings\" || ! -f \"$settings\" ]]; then",
		"  echo \"missing settings file\" >&2",
		"  exit 90",
		"fi",
		"grep -q \"<protocol>http</protocol>\" \"$settings\" || { echo \"missing http proxy\" >&2; exit 91; }",
		"grep -q \"<protocol>https</protocol>\" \"$settings\" || { echo \"missing https proxy\" >&2; exit 92; }",
		"touch \"$MAVEN_CAPTURE_FILE\"",
		"exit 0",
		"",
	}, "\n")
	if err := os.WriteFile(fakeMaven, []byte(fakeMavenScript), 0o755); err != nil {
		t.Fatalf("write fake mvn: %v", err)
	}

	wrapperPath := filepath.Join(contextDir, "mvn-wrapper.sh")
	command := exec.Command("bash", wrapperPath, "-q", "-DskipTests", "package")
	command.Env = append(
		os.Environ(),
		"MAVEN_REAL_BIN="+fakeMaven,
		"MAVEN_CAPTURE_FILE="+captureFile,
		"HTTP_PROXY=HTTP://proxy.example:8080",
		"HTTPS_PROXY=HTTPS://secure-proxy.example:8443",
		"NO_PROXY=localhost,127.0.0.1",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("wrapper execution failed: %v, output=%s", err, string(output))
	}
	if _, err := os.Stat(captureFile); err != nil {
		t.Fatalf("fake mvn was not invoked: %v, output=%s", err, string(output))
	}
}
