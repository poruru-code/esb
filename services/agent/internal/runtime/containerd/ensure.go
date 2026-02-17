// Where: services/agent/internal/runtime/containerd/ensure.go
// What: Container creation flow for the containerd runtime.
// Why: Keep runtime.go focused on lifecycle APIs and shared state.
package containerd

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/google/uuid"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/poruru/edge-serverless-box/services/agent/internal/config"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
)

func (r *Runtime) ensureResolvConf() (string, error) {
	dnsServer := resolveCNIDNSServer()
	if dnsServer == "" {
		return "", fmt.Errorf("CNI DNS server is empty")
	}
	payload := fmt.Sprintf("nameserver %s\n", dnsServer)
	if err := os.MkdirAll(filepath.Dir(r.resolvConf), 0o755); err != nil {
		return "", fmt.Errorf("create resolv.conf dir: %w", err)
	}
	if err := os.WriteFile(r.resolvConf, []byte(payload), 0o644); err != nil {
		return "", fmt.Errorf("write resolv.conf: %w", err)
	}
	return r.resolvConf, nil
}

func withResolvConf(path string) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		s.Mounts = append(s.Mounts, specs.Mount{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      path,
			Options:     []string{"rbind", "ro"},
		})
		return nil
	}
}

func memoryLimitBytes(env map[string]string) (uint64, bool) {
	if env == nil {
		return 0, false
	}
	raw, ok := env["AWS_LAMBDA_FUNCTION_MEMORY_SIZE"]
	if !ok || raw == "" {
		return 0, false
	}
	mb, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || mb == 0 {
		log.Printf("WARNING: invalid AWS_LAMBDA_FUNCTION_MEMORY_SIZE=%q", raw)
		return 0, false
	}
	const bytesPerMB uint64 = 1024 * 1024
	if mb > ^uint64(0)/bytesPerMB {
		log.Printf("WARNING: AWS_LAMBDA_FUNCTION_MEMORY_SIZE too large: %d", mb)
		return 0, false
	}
	return mb * bytesPerMB, true
}

func (r *Runtime) Ensure(ctx context.Context, req runtime.EnsureRequest) (*runtime.WorkerInfo, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)
	if r.cni == nil {
		return nil, fmt.Errorf("cni is not configured")
	}
	ownerID := strings.TrimSpace(req.OwnerID)
	if ownerID == "" {
		return nil, fmt.Errorf("owner_id is required")
	}

	image := req.Image
	if image == "" {
		registry := os.Getenv("CONTAINER_REGISTRY")
		if registry == "" {
			registry = config.DefaultContainerRegistry
		}
		baseImage, err := runtime.ResolveFunctionImageName(req.FunctionName)
		if err != nil {
			return nil, err
		}
		tag := runtime.ResolveFunctionImageTag()
		if registry != "" {
			image = fmt.Sprintf("%s/%s:%s", registry, baseImage, tag)
		} else {
			image = fmt.Sprintf("%s:%s", baseImage, tag)
		}
	}

	u := uuid.New()
	id := hex.EncodeToString(u[:4])
	containerID := fmt.Sprintf("%s-%s-%s-%s", r.brandSlug, r.env, req.FunctionName, id)

	imgObj, err := r.ensureImage(ctx, image)
	if err != nil {
		return nil, err
	}

	envList := make([]string, 0, len(req.Env))
	for k, v := range req.Env {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}

	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(imgObj),
		oci.WithEnv(envList),
	}
	if resolvPath, err := r.ensureResolvConf(); err != nil {
		log.Printf("WARNING: failed to prepare resolv.conf: %v", err)
	} else {
		specOpts = append(specOpts, withResolvConf(resolvPath))
	}
	if limitBytes, ok := memoryLimitBytes(req.Env); ok {
		specOpts = append(specOpts, oci.WithMemoryLimit(limitBytes))
	}

	snapshotter := resolveSnapshotter()
	createOpts := []containerd.NewContainerOpts{
		containerd.WithSnapshotter(snapshotter),
		containerd.WithNewSnapshot(containerID, imgObj),
		containerd.WithNewSpec(specOpts...),
		containerd.WithContainerLabels(map[string]string{
			runtime.LabelFunctionName: req.FunctionName,
			runtime.LabelCreatedBy:    runtime.ValueCreatedByAgent,
			runtime.LabelEsbEnv:       r.env,
			runtime.LabelFunctionKind: runtime.ValueFunctionKind,
			runtime.LabelOwner:        ownerID,
		}),
	}
	if runtimeName := strings.TrimSpace(os.Getenv("CONTAINERD_RUNTIME")); runtimeName != "" {
		createOpts = append(createOpts, containerd.WithRuntime(runtimeName, nil))
	}
	container, err := r.client.NewContainer(ctx, containerID, createOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	spec, err := container.Spec(ctx)
	if err != nil {
		log.Printf("WARNING: failed to get spec: %v", err)
	} else {
		if spec.Root != nil {
			log.Printf("DEBUG: Spec.Root.Path = %s", spec.Root.Path)
		}
		if spec.Process != nil {
			log.Printf("DEBUG: Spec.Process.Args = %v", spec.Process.Args)
		}
	}

	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		if delErr := container.Delete(ctx, containerd.WithSnapshotCleanup); delErr != nil {
			log.Printf("WARNING: failed to cleanup container %s: %v", containerID, delErr)
		}
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	if err := task.Start(ctx); err != nil {
		if _, delErr := task.Delete(ctx, containerd.WithProcessKill); delErr != nil {
			log.Printf("WARNING: failed to cleanup task %s: %v", containerID, delErr)
		}
		if delErr := container.Delete(ctx, containerd.WithSnapshotCleanup); delErr != nil {
			log.Printf("WARNING: failed to cleanup container %s: %v", containerID, delErr)
		}
		return nil, fmt.Errorf("failed to start task: %w", err)
	}

	netnsPath := fmt.Sprintf("/proc/%d/ns/net", task.Pid())
	result, err := r.setupCNI(ctx, containerID, netnsPath)
	if err != nil {
		_ = r.removeCNI(ctx, containerID, netnsPath)
		if _, delErr := task.Delete(ctx, containerd.WithProcessKill); delErr != nil {
			log.Printf("WARNING: failed to cleanup task %s: %v", containerID, delErr)
		}
		if delErr := container.Delete(ctx, containerd.WithSnapshotCleanup); delErr != nil {
			log.Printf("WARNING: failed to cleanup container %s: %v", containerID, delErr)
		}
		return nil, fmt.Errorf("failed to setup CNI network: %w", err)
	}

	ipAddress, err := extractIPv4(result)
	if err != nil {
		_ = r.removeCNI(ctx, containerID, netnsPath)
		if _, delErr := task.Delete(ctx, containerd.WithProcessKill); delErr != nil {
			log.Printf("WARNING: failed to cleanup task %s: %v", containerID, delErr)
		}
		if delErr := container.Delete(ctx, containerd.WithSnapshotCleanup); delErr != nil {
			log.Printf("WARNING: failed to cleanup container %s: %v", containerID, delErr)
		}
		return nil, fmt.Errorf("failed to detect container IP: %w", err)
	}

	r.accessTracker.Store(containerID, time.Now())

	return &runtime.WorkerInfo{
		ID:        containerID,
		IPAddress: ipAddress,
		Port:      8080,
		OwnerID:   ownerID,
	}, nil
}
