// Where: services/agent/internal/runtime/containerd/image.go
// What: Image pulling and unpacking helpers for the agent's containerd runtime.
// Why: Ensure images are present and unpacked for the selected snapshotter.
package containerd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/poruru/edge-serverless-box/meta"
)

// CA certificate path mounted in container
const caCertPath = meta.RootCACertPath

// ensureImage checks if the image exists in the current namespace, and pulls it if not.
func (r *Runtime) ensureImage(ctx context.Context, ref string) (containerd.Image, error) {
	snapshotter := resolveSnapshotter()
	img, err := r.client.GetImage(ctx, ref)
	if err == nil {
		if err := ensureImageUnpacked(ctx, img, snapshotter); err != nil {
			return nil, err
		}
		return img, nil
	}

	if !errdefs.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get image %s: %w", ref, err)
	}

	log.Printf("Image %s not found, pulling...", ref)

	// Create resolver with TLS configuration
	resolver, err := createTLSResolver()
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS resolver: %w", err)
	}

	img, err = r.client.Pull(ctx, ref,
		containerd.WithPullUnpack,
		containerd.WithPullSnapshotter(snapshotter),
		containerd.WithResolver(resolver),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to pull image %s: %w", ref, err)
	}

	if err := ensureImageUnpacked(ctx, img, snapshotter); err != nil {
		return nil, err
	}

	return img, nil
}

func ensureImageUnpacked(ctx context.Context, img containerd.Image, snapshotter string) error {
	if snapshotter == "" {
		return nil
	}
	unpacked, err := img.IsUnpacked(ctx, snapshotter)
	if err != nil {
		return fmt.Errorf("failed to check unpack state (%s): %w", snapshotter, err)
	}
	if unpacked {
		return nil
	}
	if err := img.Unpack(ctx, snapshotter); err != nil {
		return fmt.Errorf("failed to unpack image for %s: %w", snapshotter, err)
	}
	return nil
}

// createTLSResolver creates a docker resolver with custom CA certificate.
// Default behavior forces HTTPS even for localhost (containerd treats localhost as insecure by default).
func createTLSResolver() (remotes.Resolver, error) {
	if isRegistryInsecure() {
		return docker.NewResolver(docker.ResolverOptions{PlainHTTP: true}), nil
	}
	// Load CA certificate
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		log.Printf("Warning: CA certificate not found at %s, using system certs: %v", caCertPath, err)
		// Fall back to default resolver (system certs)
		return docker.NewResolver(docker.ResolverOptions{}), nil
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	// Create custom HTTP client with TLS config
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		},
	}

	// Create resolver with custom client and Hosts function to force HTTPS
	resolver := docker.NewResolver(docker.ResolverOptions{
		Client: httpClient,
		// Hosts function overrides default behavior for localhost
		// By default, containerd treats localhost as insecure (HTTP)
		Hosts: func(host string) ([]docker.RegistryHost, error) {
			return []docker.RegistryHost{
				{
					Host:         host,
					Scheme:       "https", // Force HTTPS
					Path:         "/v2",
					Capabilities: docker.HostCapabilityPull | docker.HostCapabilityResolve,
					Client:       httpClient,
				},
			}, nil
		},
	})

	log.Printf("TLS resolver configured with CA from %s (HTTPS forced)", caCertPath)
	return resolver, nil
}

func isRegistryInsecure() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("CONTAINER_REGISTRY_INSECURE")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
