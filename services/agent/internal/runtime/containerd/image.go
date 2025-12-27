package containerd

import (
	"context"
	"fmt"
	"log"

	"github.com/containerd/containerd"
)

// ensureImage checks if the image exists in the current namespace, and pulls it if not.
func (r *Runtime) ensureImage(ctx context.Context, ref string) (containerd.Image, error) {
	img, err := r.client.GetImage(ctx, ref)
	if err == nil {
		return img, nil
	}

	log.Printf("Image %s not found, pulling...", ref)
	img, err = r.client.Pull(ctx, ref, containerd.WithPullUnpack)
	if err != nil {
		return nil, fmt.Errorf("failed to pull image %s: %w", ref, err)
	}

	return img, nil
}
