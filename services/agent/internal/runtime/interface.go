package runtime

import (
	"context"
	"time"
)

// EnsureRequest represents the parameters for ensuring a container is running.
type EnsureRequest struct {
	FunctionName string
	Image        string
	Env          map[string]string
}

// WorkerInfo contains the identification and connection details of a managed container.
type WorkerInfo struct {
	ID        string
	IPAddress string
	Port      int // Port used for communication (especially important for containerd NAT)
}

// ContainerState represents the current state of a managed container.
// Used by Janitor to determine which containers should be cleaned up.
type ContainerState struct {
	ID            string
	FunctionName  string
	Status        string    // "RUNNING", "PAUSED", "STOPPED", "UNKNOWN"
	LastUsedAt    time.Time // Last time this container was used
	ContainerName string    // Actual docker/containerd name
	CreatedAt     time.Time // Container creation time - used for grace period in Reconciliation
	IPAddress     string    // Container IP (if available)
	Port          int       // Container port (if available)
}

// ContainerMetrics represents resource usage stats for a container.
type ContainerMetrics struct {
	ID            string
	FunctionName  string
	ContainerName string
	State         string
	MemoryCurrent uint64
	MemoryMax     uint64
	OOMEvents     uint64
	CPUUsageNS    uint64
	ExitCode      uint32
	RestartCount  uint32
	ExitTime      time.Time
	CollectedAt   time.Time
}

// ContainerRuntime defines the interface for interacting with container backends.
type ContainerRuntime interface {
	// Ensure ensures that a container for the given request is running and ready.
	Ensure(ctx context.Context, req EnsureRequest) (*WorkerInfo, error)

	// Destroy removes the specified container and its associated resources.
	Destroy(ctx context.Context, id string) error

	// Suspend pauses the container's processes.
	Suspend(ctx context.Context, id string) error

	// Resume un-suspends a previously paused container.
	Resume(ctx context.Context, id string) error

	// List returns the state of all managed containers.
	// Used by Janitor to identify idle or orphan containers.
	List(ctx context.Context) ([]ContainerState, error)

	// Metrics returns resource usage metrics for a container.
	Metrics(ctx context.Context, id string) (*ContainerMetrics, error)

	// GC performs garbage collection, cleaning up all managed containers and tasks.
	GC(ctx context.Context) error

	// Close cleans up runtime-wide resources (e.g. connections).
	Close() error
}
