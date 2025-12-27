package containerd

import (
	"context"
	"github.com/containerd/containerd"
)

// ContainerdClient defines the subset of containerd.Client methods we use,
// making it easier to mock for TDD.
type ContainerdClient interface {
	Containers(ctx context.Context, filters ...string) ([]containerd.Container, error)
	LoadContainer(ctx context.Context, id string) (containerd.Container, error)
	NewContainer(ctx context.Context, id string, opts ...containerd.NewContainerOpts) (containerd.Container, error)
	GetImage(ctx context.Context, ref string) (containerd.Image, error)
	Pull(ctx context.Context, ref string, opts ...containerd.RemoteOpt) (containerd.Image, error)
	Close() error
}

// ClientWrapper is a real implementation of ContainerdClient using containerd.Client.
type ClientWrapper struct {
	*containerd.Client
}

func (w *ClientWrapper) Containers(ctx context.Context, filters ...string) ([]containerd.Container, error) {
	return w.Client.Containers(ctx, filters...)
}

func (w *ClientWrapper) LoadContainer(ctx context.Context, id string) (containerd.Container, error) {
	return w.Client.LoadContainer(ctx, id)
}

func (w *ClientWrapper) NewContainer(ctx context.Context, id string, opts ...containerd.NewContainerOpts) (containerd.Container, error) {
	return w.Client.NewContainer(ctx, id, opts...)
}

func (w *ClientWrapper) GetImage(ctx context.Context, ref string) (containerd.Image, error) {
	return w.Client.GetImage(ctx, ref)
}

func (w *ClientWrapper) Pull(ctx context.Context, ref string, opts ...containerd.RemoteOpt) (containerd.Image, error) {
	return w.Client.Pull(ctx, ref, opts...)
}
