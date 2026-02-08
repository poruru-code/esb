// Where: services/agent/internal/runtime/containerd/cni.go
// What: CNI setup/teardown helpers and network result parsing.
// Why: Isolate networking concerns from container lifecycle orchestration.
package containerd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/containerd/go-cni"
)

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
