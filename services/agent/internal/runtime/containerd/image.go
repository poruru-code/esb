package containerd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
)

// CA certificate path mounted in container
const caCertPath = "/usr/local/share/ca-certificates/esb-rootCA.crt"

// ensureImage checks if the image exists in the current namespace, and pulls it if not.
func (r *Runtime) ensureImage(ctx context.Context, ref string) (containerd.Image, error) {
	img, err := r.client.GetImage(ctx, ref)
	if err == nil {
		return img, nil
	}

	log.Printf("Image %s not found, pulling...", ref)

	// Create resolver with TLS configuration
	resolver, err := createTLSResolver()
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS resolver: %w", err)
	}

	img, err = r.client.Pull(ctx, ref,
		containerd.WithPullUnpack,
		containerd.WithResolver(resolver),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to pull image %s: %w", ref, err)
	}

	return img, nil
}

// createTLSResolver creates a docker resolver with custom CA certificate
// Forces HTTPS even for localhost (containerd treats localhost as insecure by default)
func createTLSResolver() (remotes.Resolver, error) {
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
