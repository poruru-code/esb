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

type GatewayWaiter interface {
	Wait(ctx state.Context) error
}

type gatewayWaiter struct {
	client   *http.Client
	timeout  time.Duration
	interval time.Duration
}

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

	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := w.client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(w.interval)
	}

	return fmt.Errorf("gateway failed to start")
}
