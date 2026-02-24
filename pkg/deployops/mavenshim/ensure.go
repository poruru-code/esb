package mavenshim

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	proxymaven "github.com/poruru-code/esb/pkg/proxy/maven"
)

const (
	mavenShimImagePrefix = "esb-maven-shim"
	mavenShimTagSchema   = "v2"
	lockAcquireTimeout   = 2 * time.Minute
	lockRetryInterval    = 200 * time.Millisecond
	staleLockThreshold   = 5 * time.Minute
)

//go:embed assets/*
var mavenShimAssets embed.FS

var mavenShimAssetFingerprint = mustComputeMavenShimAssetFingerprint()

type CommandRunner interface {
	Run(cmd []string) error
}

type EnsureInput struct {
	BaseImage    string
	HostRegistry string
	NoCache      bool
	Runner       CommandRunner
	ImageExists  func(imageRef string) bool
}

type EnsureResult struct {
	ShimImage string
}

func EnsureImage(input EnsureInput) (EnsureResult, error) {
	baseRef := strings.TrimSpace(input.BaseImage)
	if baseRef == "" {
		return EnsureResult{}, fmt.Errorf("maven base image reference is empty")
	}
	if input.Runner == nil {
		return EnsureResult{}, fmt.Errorf("command runner is nil")
	}

	shimImage := deriveShimImageTag(baseRef)

	hostRegistry := strings.TrimSuffix(strings.TrimSpace(input.HostRegistry), "/")
	shimRef := shimImage
	if hostRegistry != "" {
		shimRef = hostRegistry + "/" + shimImage
	}

	lockPath := shimLockPath(shimRef)
	releaseLock, err := acquireShimLock(lockPath, lockAcquireTimeout, lockRetryInterval, staleLockThreshold)
	if err != nil {
		return EnsureResult{}, err
	}
	defer releaseLock()

	imageExists := input.ImageExists
	if imageExists == nil {
		imageExists = dockerImageExists
	}

	if input.NoCache || !imageExists(shimRef) {
		if err := validateProxyEnv(); err != nil {
			return EnsureResult{}, err
		}
		contextDir, cleanup, err := materializeBuildContext()
		if err != nil {
			return EnsureResult{}, err
		}
		defer cleanup()

		buildCmd := buildxBuildCommandWithBuildArgs(
			shimRef,
			filepath.Join(contextDir, "Dockerfile"),
			contextDir,
			input.NoCache,
			map[string]string{
				"BASE_MAVEN_IMAGE": baseRef,
			},
		)
		if err := input.Runner.Run(buildCmd); err != nil {
			return EnsureResult{}, fmt.Errorf("build maven shim image %s from %s: %w", shimRef, baseRef, err)
		}
	}

	if hostRegistry != "" {
		if err := input.Runner.Run([]string{"docker", "push", shimRef}); err != nil {
			return EnsureResult{}, fmt.Errorf("push maven shim image %s: %w", shimRef, err)
		}
	}

	return EnsureResult{ShimImage: shimRef}, nil
}

func deriveShimImageTag(baseRef string) string {
	hashInput := strings.Join(
		[]string{mavenShimTagSchema, baseRef, mavenShimAssetFingerprint},
		"\n",
	)
	hash := sha256.Sum256([]byte(hashInput))
	shortHash := hex.EncodeToString(hash[:])[:16]
	return fmt.Sprintf("%s:%s", mavenShimImagePrefix, shortHash)
}

func mustComputeMavenShimAssetFingerprint() string {
	files := []string{
		"assets/Dockerfile",
		"assets/mvn-wrapper.sh",
	}
	digest := sha256.New()
	for _, file := range files {
		content, err := mavenShimAssets.ReadFile(file)
		if err != nil {
			panic(fmt.Sprintf("read maven shim asset %s: %v", file, err))
		}
		_, _ = digest.Write([]byte(file))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write(content)
		_, _ = digest.Write([]byte{0})
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func validateProxyEnv() error {
	env := map[string]string{
		"HTTP_PROXY":  os.Getenv("HTTP_PROXY"),
		"http_proxy":  os.Getenv("http_proxy"),
		"HTTPS_PROXY": os.Getenv("HTTPS_PROXY"),
		"https_proxy": os.Getenv("https_proxy"),
	}
	if _, err := proxymaven.ResolveEndpointsFromEnv(env); err != nil {
		return fmt.Errorf("invalid proxy configuration for maven shim build: %w", err)
	}
	return nil
}

func materializeBuildContext() (contextDir string, cleanup func(), err error) {
	contextDir, err = os.MkdirTemp("", "esb-maven-shim-*")
	if err != nil {
		return "", nil, fmt.Errorf("create maven shim build context: %w", err)
	}
	cleanup = func() {
		_ = os.RemoveAll(contextDir)
	}
	if err := writeAsset(contextDir, "Dockerfile", 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := writeAsset(contextDir, "mvn-wrapper.sh", 0o755); err != nil {
		cleanup()
		return "", nil, err
	}
	return contextDir, cleanup, nil
}

func writeAsset(contextDir, name string, mode fs.FileMode) error {
	content, err := mavenShimAssets.ReadFile(filepath.ToSlash(filepath.Join("assets", name)))
	if err != nil {
		return fmt.Errorf("read maven shim asset %s: %w", name, err)
	}
	path := filepath.Join(contextDir, name)
	if err := os.WriteFile(path, content, mode); err != nil {
		return fmt.Errorf("write maven shim asset %s: %w", name, err)
	}
	return nil
}

func buildxBuildCommandWithBuildArgs(
	tag, dockerfile, contextDir string,
	noCache bool,
	buildArgs map[string]string,
) []string {
	cmd := []string{
		"docker",
		"buildx",
		"build",
		"--platform",
		"linux/amd64",
		"--load",
		"--pull",
	}
	if noCache {
		cmd = append(cmd, "--no-cache")
	}
	cmd = appendProxyBuildArgs(cmd)
	if len(buildArgs) > 0 {
		keys := make([]string, 0, len(buildArgs))
		for key := range buildArgs {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := strings.TrimSpace(buildArgs[key])
			if value == "" {
				continue
			}
			cmd = append(cmd, "--build-arg", key+"="+value)
		}
	}
	cmd = append(cmd, "--tag", tag, "--file", dockerfile, contextDir)
	return cmd
}

func appendProxyBuildArgs(cmd []string) []string {
	type proxyEnvPair struct {
		upper string
		lower string
	}
	pairs := []proxyEnvPair{
		{upper: "HTTP_PROXY", lower: "http_proxy"},
		{upper: "HTTPS_PROXY", lower: "https_proxy"},
		{upper: "NO_PROXY", lower: "no_proxy"},
	}
	for _, pair := range pairs {
		value := strings.TrimSpace(os.Getenv(pair.upper))
		if value == "" {
			value = strings.TrimSpace(os.Getenv(pair.lower))
		}
		if value == "" {
			continue
		}
		cmd = append(cmd, "--build-arg", pair.upper+"="+value)
		cmd = append(cmd, "--build-arg", pair.lower+"="+value)
	}
	return cmd
}

func dockerImageExists(imageRef string) bool {
	cmd := exec.Command("docker", "image", "inspect", imageRef)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

type shimLockMetadata struct {
	PID       int    `json:"pid"`
	CreatedAt string `json:"created_at"`
}

func shimLockPath(shimRef string) string {
	sum := sha256.Sum256([]byte(shimRef))
	return filepath.Join(os.TempDir(), fmt.Sprintf("esb-maven-shim-%x.lock", sum[:8]))
}

func acquireShimLock(
	lockPath string,
	timeout time.Duration,
	retryInterval time.Duration,
	staleThreshold time.Duration,
) (func(), error) {
	startedAt := time.Now()
	for {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			metadata := shimLockMetadata{
				PID:       os.Getpid(),
				CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			}
			if writeErr := json.NewEncoder(file).Encode(metadata); writeErr != nil {
				_ = file.Close()
				_ = os.Remove(lockPath)
				return nil, fmt.Errorf("initialize maven shim lock %s: %w", lockPath, writeErr)
			}
			if closeErr := file.Close(); closeErr != nil {
				_ = os.Remove(lockPath)
				return nil, fmt.Errorf("close maven shim lock %s: %w", lockPath, closeErr)
			}
			return func() {
				_ = os.Remove(lockPath)
			}, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("create maven shim lock %s: %w", lockPath, err)
		}

		evicted, evictErr := evictStaleShimLock(lockPath, staleThreshold)
		if evictErr != nil {
			return nil, evictErr
		}
		if evicted {
			continue
		}

		if time.Since(startedAt) >= timeout {
			return nil, fmt.Errorf("timeout acquiring maven shim lock: %s", lockPath)
		}
		time.Sleep(retryInterval)
	}
}

func evictStaleShimLock(lockPath string, staleThreshold time.Duration) (bool, error) {
	if staleThreshold <= 0 {
		return false, nil
	}
	info, err := os.Stat(lockPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspect maven shim lock %s: %w", lockPath, err)
	}
	if time.Since(info.ModTime()) < staleThreshold {
		return false, nil
	}
	if err := os.Remove(lockPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("remove stale maven shim lock %s: %w", lockPath, err)
	}
	return true, nil
}
