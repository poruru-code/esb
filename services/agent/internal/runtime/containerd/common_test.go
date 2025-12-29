package containerd

import (
	"context"
	"fmt"

	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/go-cni"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/mock"
)

// MockClient is a mock for ContainerdClient
type MockClient struct {
	mock.Mock
}

func (m *MockClient) Containers(ctx context.Context, filters ...string) ([]containerd.Container, error) {
	args := m.Called(ctx, filters)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]containerd.Container), args.Error(1)
}

func (m *MockClient) LoadContainer(ctx context.Context, id string) (containerd.Container, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(containerd.Container), args.Error(1)
}

func (m *MockClient) NewContainer(ctx context.Context, id string, opts ...containerd.NewContainerOpts) (containerd.Container, error) {
	args := m.Called(ctx, id, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(containerd.Container), args.Error(1)
}

func (m *MockClient) GetImage(ctx context.Context, ref string) (containerd.Image, error) {
	args := m.Called(ctx, ref)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(containerd.Image), args.Error(1)
}

func (m *MockClient) Pull(ctx context.Context, ref string, opts ...containerd.RemoteOpt) (containerd.Image, error) {
	args := m.Called(ctx, ref, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(containerd.Image), args.Error(1)
}

func (m *MockClient) Close() error {
	return m.Called().Error(0)
}

// MockCNI is a mock for go-cni.CNI interface
type MockCNI struct {
	mock.Mock
}

func (m *MockCNI) Setup(ctx context.Context, id, path string, opts ...cni.NamespaceOpts) (*cni.Result, error) {
	args := m.Called(ctx, id, path, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*cni.Result), args.Error(1)
}

func (m *MockCNI) SetupSerially(ctx context.Context, id, path string, opts ...cni.NamespaceOpts) (*cni.Result, error) {
	args := m.Called(ctx, id, path, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*cni.Result), args.Error(1)
}

func (m *MockCNI) Remove(ctx context.Context, id, path string, opts ...cni.NamespaceOpts) error {
	args := m.Called(ctx, id, path, opts)
	return args.Error(0)
}

func (m *MockCNI) Load(opts ...cni.Opt) error {
	return m.Called(opts).Error(0)
}

func (m *MockCNI) Status() error {
	return m.Called().Error(0)
}

func (m *MockCNI) GetConfig() *cni.ConfigResult {
	args := m.Called()
	return args.Get(0).(*cni.ConfigResult)
}

func (m *MockCNI) Check(ctx context.Context, id, path string, opts ...cni.NamespaceOpts) error {
	return m.Called(ctx, id, path, opts).Error(0)
}

// MockContainer mocks containerd.Container
type MockContainer struct {
	mock.Mock
	containerd.Container
}

func (m *MockContainer) ID() string {
	return m.Called().String(0)
}

func (m *MockContainer) Task(ctx context.Context, attach cio.Attach) (containerd.Task, error) {
	args := m.Called(ctx, attach)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(containerd.Task), args.Error(1)
}

func (m *MockContainer) NewTask(ctx context.Context, creator cio.Creator, opts ...containerd.NewTaskOpts) (containerd.Task, error) {
	args := m.Called(ctx, creator, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(containerd.Task), args.Error(1)
}

func (m *MockContainer) Labels(ctx context.Context) (map[string]string, error) {
	args := m.Called(ctx)
	return args.Get(0).(map[string]string), args.Error(1)
}

func (m *MockContainer) Info(ctx context.Context, opts ...containerd.InfoOpts) (containers.Container, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return containers.Container{}, args.Error(1)
	}
	return args.Get(0).(containers.Container), args.Error(1)
}

// MockTask mocks containerd.Task
type MockTask struct {
	mock.Mock
	containerd.Task
}

func (m *MockTask) Start(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *MockTask) Pid() uint32 {
	return m.Called().Get(0).(uint32)
}

func (m *MockTask) Status(ctx context.Context) (containerd.Status, error) {
	args := m.Called(ctx)
	return args.Get(0).(containerd.Status), args.Error(1)
}

// MockImage mocks containerd.Image
type MockImage struct {
	mock.Mock
	containerd.Image
}

func (m *MockImage) Name() string {
	return "mock-image"
}

func (m *MockImage) Target() v1.Descriptor {
	return v1.Descriptor{}
}

func (m *MockImage) Labels() (map[string]string, error) {
	return nil, nil
}

func (m *MockImage) Config(ctx context.Context) (v1.Descriptor, error) {
	return v1.Descriptor{}, nil
}

// MockContentStore stubs content.Store
type MockContentStore struct {
	mock.Mock
}

func (m *MockContentStore) Info(ctx context.Context, dgst digest.Digest) (content.Info, error) {
	return content.Info{}, nil
}

func (m *MockContentStore) Update(ctx context.Context, info content.Info, fieldpaths ...string) (content.Info, error) {
	return content.Info{}, nil
}

func (m *MockContentStore) Walk(ctx context.Context, fn content.WalkFunc, filters ...string) error {
	return nil
}

func (m *MockContentStore) Delete(ctx context.Context, dgst digest.Digest) error {
	return nil
}

func (m *MockContentStore) ReaderAt(ctx context.Context, desc v1.Descriptor) (content.ReaderAt, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *MockContentStore) Status(ctx context.Context, ref string) (content.Status, error) {
	return content.Status{}, nil
}

func (m *MockContentStore) ListStatuses(ctx context.Context, filters ...string) ([]content.Status, error) {
	return nil, nil
}

func (m *MockContentStore) Abort(ctx context.Context, ref string) error {
	return nil
}

func (m *MockContentStore) Writer(ctx context.Context, opts ...content.WriterOpt) (content.Writer, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *MockImage) ContentStore() content.Store {
	return &MockContentStore{}
}

func (m *MockImage) Metadata() images.Image {
	return images.Image{}
}

func (m *MockImage) Platform() v1.Platform {
	return v1.Platform{}
}

func (m *MockImage) Size(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *MockImage) Usage(ctx context.Context, usages ...containerd.UsageOpt) (int64, error) {
	return 0, nil
}

func (m *MockContainer) Delete(ctx context.Context, opts ...containerd.DeleteOpts) error {
	args := m.Called(ctx, opts)
	return args.Error(0)
}

func (m *MockTask) Delete(ctx context.Context, opts ...containerd.ProcessDeleteOpts) (*containerd.ExitStatus, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*containerd.ExitStatus), args.Error(1)
}

func (m *MockTask) Kill(ctx context.Context, signal syscall.Signal, opts ...containerd.KillOpts) error {
	args := m.Called(ctx, signal, opts)
	return args.Error(0)
}

func (m *MockTask) Pause(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockTask) Resume(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}
