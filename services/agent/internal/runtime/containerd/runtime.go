package containerd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	cgroup1stats "github.com/containerd/cgroups/v3/cgroup1/stats"
	cgroup2stats "github.com/containerd/cgroups/v3/cgroup2/stats"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/go-cni"
	"github.com/containerd/typeurl/v2"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
)

type Runtime struct {
	client        ContainerdClient
	cni           cni.CNI
	cniMu         sync.Mutex // serialize CNI operations to avoid bridge races
	namespace     string
	accessTracker sync.Map // map[containerID]time.Time - tracks last access time
}

// NewRuntime creates a new containerd runtime with CNI networking.
func NewRuntime(client ContainerdClient, cniPlugin cni.CNI, namespace string) *Runtime {
	return &Runtime{
		client:    client,
		cni:       cniPlugin,
		namespace: namespace,
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

func mapTaskState(status containerd.ProcessStatus) string {
	switch status {
	case containerd.Running:
		return "RUNNING"
	case containerd.Paused:
		return "PAUSED"
	case containerd.Stopped:
		return "STOPPED"
	default:
		return "UNKNOWN"
	}
}

func extractFunctionName(containerID string) string {
	if !strings.HasPrefix(containerID, runtime.ContainerNamePrefix) {
		return ""
	}
	trimmed := strings.TrimPrefix(containerID, runtime.ContainerNamePrefix)
	parts := strings.Split(trimmed, "-")
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts[:len(parts)-1], "-")
}

func extractTaskMetrics(metric *types.Metric) (uint64, uint64, uint64, uint64, error) {
	if metric == nil || metric.Data == nil {
		return 0, 0, 0, 0, fmt.Errorf("metrics data is empty")
	}

	unpacked, err := typeurl.UnmarshalAny(metric.Data)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to unmarshal metrics: %w", err)
	}

	switch data := unpacked.(type) {
	case *cgroup1stats.Metrics:
		var memoryCurrent uint64
		var memoryMax uint64
		if data.Memory != nil {
			memoryCurrent = data.Memory.RSS
			if data.Memory.Usage != nil {
				memoryMax = data.Memory.Usage.Limit
			}
		}
		var oomEvents uint64
		if data.MemoryOomControl != nil {
			oomEvents = data.MemoryOomControl.OomKill
		}
		var cpuUsageNS uint64
		if data.CPU != nil && data.CPU.Usage != nil {
			cpuUsageNS = data.CPU.Usage.Total
		}
		return memoryCurrent, memoryMax, oomEvents, cpuUsageNS, nil
	case *cgroup2stats.Metrics:
		var memoryCurrent uint64
		var memoryMax uint64
		if data.Memory != nil {
			memoryCurrent = data.Memory.Usage
			memoryMax = data.Memory.UsageLimit
		}
		var oomEvents uint64
		if data.MemoryEvents != nil {
			if data.MemoryEvents.OomKill > 0 {
				oomEvents = data.MemoryEvents.OomKill
			} else {
				oomEvents = data.MemoryEvents.Oom
			}
		}
		var cpuUsageNS uint64
		if data.CPU != nil {
			cpuUsageNS = data.CPU.UsageUsec * 1000
		}
		return memoryCurrent, memoryMax, oomEvents, cpuUsageNS, nil
	default:
		return 0, 0, 0, 0, fmt.Errorf("unsupported metrics type %T", unpacked)
	}
}

func (r *Runtime) Ensure(ctx context.Context, req runtime.EnsureRequest) (*runtime.WorkerInfo, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)
	if r.cni == nil {
		return nil, fmt.Errorf("cni is not configured")
	}

	// Phase 4-1: Factory behavior. Always create a new container.
	image := req.Image
	if image == "" {
		// Phase 5 Step 0: Support container registry
		registry := os.Getenv("CONTAINER_REGISTRY")
		if registry != "" {
			image = fmt.Sprintf("%s/%s:latest", registry, req.FunctionName)
		} else {
			// Fallback to local image (backward compatibility)
			image = fmt.Sprintf("%s:latest", req.FunctionName)
		}
	}

	containerID := fmt.Sprintf("%s%s-%d", runtime.ContainerNamePrefix, req.FunctionName, time.Now().UnixNano())

	// 1. Ensure image (only for Cold Start)
	imgObj, err := r.ensureImage(ctx, image)
	if err != nil {
		return nil, err
	}

	// Make env list
	envList := make([]string, 0, len(req.Env))
	for k, v := range req.Env {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}

	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(imgObj), // Apply image config (ENTRYPOINT, CMD, ENV, WORKDIR)
		oci.WithEnv(envList),        // Override with custom env
	}
	if limitBytes, ok := memoryLimitBytes(req.Env); ok {
		specOpts = append(specOpts, oci.WithMemoryLimit(limitBytes))
	}

	// 2. Create Container with CNI networking
	createOpts := []containerd.NewContainerOpts{
		containerd.WithSnapshotter("overlayfs"),
		containerd.WithNewSnapshot(containerID, imgObj),
		containerd.WithNewSpec(
			specOpts...,
		),
		containerd.WithContainerLabels(map[string]string{
			runtime.LabelFunctionName: req.FunctionName,
			runtime.LabelCreatedBy:    runtime.ValueCreatedByAgent,
		}),
	}
	if runtimeName := strings.TrimSpace(os.Getenv("CONTAINERD_RUNTIME")); runtimeName != "" {
		// Allow runtime override (e.g. firecracker shim) without changing agent mode.
		createOpts = append(createOpts, containerd.WithRuntime(runtimeName, nil))
	}
	container, err := r.client.NewContainer(ctx, containerID, createOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// DEBUG: Dump spec to verify configuration
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

	// 3. Create and Start Task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	if err := task.Start(ctx); err != nil {
		task.Delete(ctx, containerd.WithProcessKill)
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return nil, fmt.Errorf("failed to start task: %w", err)
	}

	netnsPath := fmt.Sprintf("/proc/%d/ns/net", task.Pid())
	result, err := r.setupCNI(ctx, containerID, netnsPath)
	if err != nil {
		_ = r.removeCNI(ctx, containerID, netnsPath)
		task.Delete(ctx, containerd.WithProcessKill)
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return nil, fmt.Errorf("failed to setup CNI network: %w", err)
	}

	ipAddress, err := extractIPv4(result)
	if err != nil {
		_ = r.removeCNI(ctx, containerID, netnsPath)
		task.Delete(ctx, containerd.WithProcessKill)
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return nil, fmt.Errorf("failed to detect container IP: %w", err)
	}

	// Record access time for Janitor
	r.accessTracker.Store(containerID, time.Now())

	// CNI Mode: Lambda is accessible at container IP:8080
	return &runtime.WorkerInfo{
		ID:        containerID,
		IPAddress: ipAddress,
		Port:      8080,
	}, nil
}

func extractIPv4(result *cni.Result) (string, error) {
	if result == nil {
		return "", fmt.Errorf("CNI result is nil")
	}
	for _, cfg := range result.Interfaces {
		for _, ipCfg := range cfg.IPConfigs {
			if ip := ipCfg.IP.To4(); ip != nil {
				return ip.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no IPv4 address in CNI result")
}

func (r *Runtime) setupCNI(ctx context.Context, id, netnsPath string) (*cni.Result, error) {
	var lastErr error
	backoff := 100 * time.Millisecond
	for attempt := 0; attempt < 5; attempt++ {
		r.cniMu.Lock()
		result, err := r.cni.Setup(ctx, id, netnsPath)
		r.cniMu.Unlock()
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !strings.Contains(err.Error(), "Link not found") {
			break
		}
		if ctx.Err() != nil {
			break
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
		if backoff < 800*time.Millisecond {
			backoff *= 2
		}
	}
	return nil, lastErr
}

func (r *Runtime) removeCNI(ctx context.Context, id, netnsPath string) error {
	r.cniMu.Lock()
	defer r.cniMu.Unlock()
	return r.cni.Remove(ctx, id, netnsPath)
}

func (r *Runtime) Destroy(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to load container %s: %w", id, err)
	}

	// Delete task if exists
	task, err := container.Task(ctx, nil)
	if err == nil {
		if r.cni != nil {
			netnsPath := fmt.Sprintf("/proc/%d/ns/net", task.Pid())
			if err := r.removeCNI(ctx, id, netnsPath); err != nil {
				log.Printf("WARNING: failed to remove CNI network for %s: %v", id, err)
			}
		}
		_, _ = task.Delete(ctx, containerd.WithProcessKill)
	}

	// Delete container
	if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
		return fmt.Errorf("failed to delete container %s: %w", id, err)
	}

	// Remove from accessTracker
	r.accessTracker.Delete(id)

	return nil
}

func (r *Runtime) Suspend(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to load container %s: %w", id, err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get task for container %s: %w", id, err)
	}

	if err := task.Pause(ctx); err != nil {
		return fmt.Errorf("failed to pause task for container %s: %w", id, err)
	}

	return nil
}

func (r *Runtime) Resume(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to load container %s: %w", id, err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get task for container %s: %w", id, err)
	}

	if err := task.Resume(ctx); err != nil {
		return fmt.Errorf("failed to resume task for container %s: %w", id, err)
	}

	// Record access time for Janitor
	r.accessTracker.Store(id, time.Now())

	return nil
}

func (r *Runtime) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

func (r *Runtime) Metrics(ctx context.Context, id string) (*runtime.ContainerMetrics, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to load container %s: %w", id, err)
	}

	functionName := ""
	if labels, err := container.Labels(ctx); err == nil {
		functionName = labels[runtime.LabelFunctionName]
	}
	if functionName == "" {
		functionName = extractFunctionName(id)
	}

	result := &runtime.ContainerMetrics{
		ID:            id,
		ContainerName: id,
		FunctionName:  functionName,
		State:         "UNKNOWN",
		CollectedAt:   time.Now(),
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		if errdefs.IsNotFound(err) {
			result.State = "STOPPED"
			return result, nil
		}
		return nil, fmt.Errorf("failed to get task for container %s: %w", id, err)
	}

	status, err := task.Status(ctx)
	if err == nil {
		result.State = mapTaskState(status.Status)
		result.ExitCode = status.ExitStatus
		result.ExitTime = status.ExitTime
	}

	metric, err := task.Metrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics for container %s: %w", id, err)
	}

	memoryCurrent, memoryMax, oomEvents, cpuUsageNS, err := extractTaskMetrics(metric)
	if err != nil {
		return nil, fmt.Errorf("failed to parse metrics for container %s: %w", id, err)
	}

	result.MemoryCurrent = memoryCurrent
	result.MemoryMax = memoryMax
	result.OOMEvents = oomEvents
	result.CPUUsageNS = cpuUsageNS

	return result, nil
}

// List returns the state of all managed containers.
// Used by Janitor to identify idle or orphan containers.
func (r *Runtime) List(ctx context.Context) ([]runtime.ContainerState, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	// Get all containers managed by ESB
	containers, err := r.client.Containers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var states []runtime.ContainerState
	for _, c := range containers {
		containerID := c.ID()

		// Skip containers not managed by our runtime (check for lambda- prefix)
		if !strings.HasPrefix(containerID, runtime.ContainerNamePrefix) {
			continue
		}

		// Get container info for timestamps and labels.
		info, infoErr := c.Info(ctx)
		createdAt := time.Time{}
		functionName := ""
		if infoErr == nil {
			createdAt = info.CreatedAt
			functionName = info.Labels[runtime.LabelFunctionName]
		} else {
			labels, err := c.Labels(ctx)
			if err == nil {
				functionName = labels[runtime.LabelFunctionName]
			}
		}
		if createdAt.IsZero() {
			createdAt = time.Now()
		}

		// Get task status
		status := "UNKNOWN"
		task, err := c.Task(ctx, nil)
		if err == nil {
			s, err := task.Status(ctx)
			if err == nil {
				switch s.Status {
				case containerd.Running:
					status = "RUNNING"
				case containerd.Paused:
					status = "PAUSED"
				case containerd.Stopped:
					status = "STOPPED"
				default:
					status = "UNKNOWN"
				}
			}
		} else {
			status = "STOPPED" // No task means container is stopped
		}

		// Get last access time from tracker
		lastUsedAt := createdAt
		if val, ok := r.accessTracker.Load(containerID); ok {
			lastUsedAt = val.(time.Time)
		}

		states = append(states, runtime.ContainerState{
			ID:           containerID,
			FunctionName: functionName,
			Status:       status,
			LastUsedAt:   lastUsedAt,
			ContainerName: containerID,
			CreatedAt:     createdAt,
		})
	}

	return states, nil
}
