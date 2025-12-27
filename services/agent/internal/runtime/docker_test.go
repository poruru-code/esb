package runtime_test

import (
	"context"
	"io"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockDockerClient mocks the Docker API client
type MockDockerClient struct {
	mock.Mock
}

func (m *MockDockerClient) ContainerList(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error) {
	args := m.Called(ctx, options)
	return args.Get(0).([]types.Container), args.Error(1)
}

func (m *MockDockerClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error) {
	// v1.Platform is from opencontainers/image-spec, but client interface uses it.
	// We can use interface{} for platform to simplify if needed, but better stick to signature if we implement interface.
	// Since we are not implementing the full CommonAPIClient interface here (it's huge),
	// we will define an interface in runtime/docker.go that covers what we need.
	// For this test, we assume runtime.DockerClient interface.

	args := m.Called(ctx, config, hostConfig, networkingConfig, platform, containerName)
	return args.Get(0).(container.CreateResponse), args.Error(1)
}

func (m *MockDockerClient) ContainerStart(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
	args := m.Called(ctx, containerID, options)
	return args.Error(0)
}

func (m *MockDockerClient) NetworkConnect(ctx context.Context, networkID, containerID string, config *network.EndpointSettings) error {
	args := m.Called(ctx, networkID, containerID, config)
	return args.Error(0)
}

func (m *MockDockerClient) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	args := m.Called(ctx, containerID)
	return args.Get(0).(types.ContainerJSON), args.Error(1)
}

func (m *MockDockerClient) ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error {
	args := m.Called(ctx, containerID, options)
	return args.Error(0)
}

func (m *MockDockerClient) ImagePull(ctx context.Context, ref string, options types.ImagePullOptions) (io.ReadCloser, error) {
	args := m.Called(ctx, ref, options)
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

// Ensure MockDockerClient satisfies runtime.DockerClient interface (defined later)
// var _ runtime.DockerClient = (*MockDockerClient)(nil)

func TestDockerRuntime_EnsureContainer_New(t *testing.T) {
	mockClient := new(MockDockerClient)
	rt := runtime.NewDockerRuntime(mockClient, "edge-serverless-box_default")

	ctx := context.Background()
	fnName := "test-func"
	image := "test-image"
	env := map[string]string{"KEY": "ALUE"}

	// 1. List: Not found
	mockClient.On("ContainerList", ctx, mock.Anything).Return([]types.Container{}, nil)

	// 2. Create
	mockClient.On("ContainerCreate", ctx, mock.MatchedBy(func(c *container.Config) bool {
		return c.Image == image && c.Env[0] == "KEY=ALUE" && c.Labels["esb_function"] == fnName
	}), mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(container.CreateResponse{ID: "new-id"}, nil)

	// 3. Start
	mockClient.On("ContainerStart", ctx, "new-id", mock.Anything).Return(nil)

	// 4. Inspect (to get IP)
	mockClient.On("ContainerInspect", ctx, "new-id").Return(types.ContainerJSON{
		NetworkSettings: &types.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"edge-serverless-box_default": {IPAddress: "10.0.0.2"},
			},
		},
	}, nil)

	// Execute
	info, err := rt.EnsureContainer(ctx, fnName, image, env)

	assert.NoError(t, err)
	assert.Equal(t, "new-id", info.ID)
	assert.Equal(t, "10.0.0.2", info.IPAddress)

	// Verify Network Config was applied (either via Create or Connect)
	// In this mock, we assume Create handles it via networkingConfig
	mockClient.AssertExpectations(t)
}

func TestDockerRuntime_EnsureContainer_Exists(t *testing.T) {
	mockClient := new(MockDockerClient)
	rt := runtime.NewDockerRuntime(mockClient, "edge-serverless-box_default")

	ctx := context.Background()
	fnName := "existing-func"

	// 1. List: Found
	mockClient.On("ContainerList", ctx, mock.Anything).Return([]types.Container{
		{
			ID:     "existing-id",
			Names:  []string{"/lambda-existing-func-1234"},
			Labels: map[string]string{"esb_function": fnName},
			State:  "running",
		},
	}, nil)

	// 2. Inspect
	mockClient.On("ContainerInspect", ctx, "existing-id").Return(types.ContainerJSON{
		NetworkSettings: &types.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"edge-serverless-box_default": {IPAddress: "10.0.0.3"},
			},
		},
	}, nil)

	// Execute
	info, err := rt.EnsureContainer(ctx, fnName, "", nil)

	assert.NoError(t, err)
	assert.Equal(t, "existing-id", info.ID)
	assert.Equal(t, "10.0.0.3", info.IPAddress)

	mockClient.AssertExpectations(t)
}
