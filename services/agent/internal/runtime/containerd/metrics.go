// Where: services/agent/internal/runtime/containerd/metrics.go
// What: Runtime metrics extraction and task-state mapping.
// Why: Keep metrics concerns separated from runtime lifecycle operations.
package containerd

import (
	"context"
	"fmt"
	"time"

	cgroup1stats "github.com/containerd/cgroups/v3/cgroup1/stats"
	cgroup2stats "github.com/containerd/cgroups/v3/cgroup2/stats"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/typeurl/v2"
	"github.com/poruru-code/esb/services/agent/internal/runtime"
)

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
		return nil, fmt.Errorf("function name label is required for container %s", id)
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
