// Where: cli/internal/app/wait.go
// What: Gateway readiness waiting helpers.
// Why: Avoid flakey E2E by waiting for gateway health.
package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// GatewayWaiter defines the interface for waiting until the gateway is ready.
// This prevents flaky E2E tests by ensuring the gateway is healthy before proceeding.
type GatewayWaiter interface {
	Wait(ctx state.Context) error
}

// gatewayWaiter implements GatewayWaiter using HTTP health checks.
type gatewayWaiter struct {
	client   *http.Client
	timeout  time.Duration
	interval time.Duration
}

// NewGatewayWaiter creates a GatewayWaiter that polls the gateway's health
// endpoint with TLS certificate verification disabled for local development.
func NewGatewayWaiter() GatewayWaiter {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return gatewayWaiter{
		client: &http.Client{
			Transport: transport,
			Timeout:   1 * time.Second,
		},
		timeout:  60 * time.Second,
		interval: 1 * time.Second,
	}
}

// Wait polls the gateway's health endpoint until it returns 200 OK
// or the timeout is reached. Uses ESB_PORT_GATEWAY_HTTPS for the port.
func (w gatewayWaiter) Wait(_ state.Context) error {
	if w.client == nil {
		return fmt.Errorf("gateway waiter client not configured")
	}
	port := strings.TrimSpace(os.Getenv("ESB_PORT_GATEWAY_HTTPS"))
	if port == "" {
		port = "443"
	}
	url := fmt.Sprintf("https://localhost:%s/health", port)
	deadline := time.Now().Add(w.timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := w.client.Do(req)
		if err != nil {
			lastErr = err
		} else {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("status code %d", resp.StatusCode)
		}
		time.Sleep(w.interval)
	}

	if lastErr != nil {
		return fmt.Errorf("gateway failed to start: %v", lastErr)
	}
	return fmt.Errorf("gateway failed to start (timeout)")
}
