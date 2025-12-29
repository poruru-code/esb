package docker

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockDockerClient struct {
	mock.Mock
}

func (m *MockDockerClient) ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
	args := m.Called(ctx, options)
	return args.Get(0).([]container.Summary), args.Error(1)
}

func (m *MockDockerClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error) {
	args := m.Called(ctx, config, hostConfig, networkingConfig, platform, containerName)
	return args.Get(0).(container.CreateResponse), args.Error(1)
}

func (m *MockDockerClient) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	args := m.Called(ctx, containerID, options)
	return args.Error(0)
}

func (m *MockDockerClient) NetworkConnect(ctx context.Context, networkID, containerID string, config *network.EndpointSettings) error {
	args := m.Called(ctx, networkID, containerID, config)
	return args.Error(0)
}

func (m *MockDockerClient) ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error) {
	args := m.Called(ctx, containerID)
	return args.Get(0).(container.InspectResponse), args.Error(1)
}

func (m *MockDockerClient) ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error {
	args := m.Called(ctx, containerID, options)
	return args.Error(0)
}

func (m *MockDockerClient) ImagePull(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
	args := m.Called(ctx, ref, options)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func TestRuntime_Ensure(t *testing.T) {
	mockClient := new(MockDockerClient)
	rt := NewRuntime(mockClient, "esb-net")

	ctx := context.Background()
	req := runtime.EnsureRequest{
		FunctionName: "test-func",
		Image:        "test-image",
	}

	mockClient.On("ImagePull", ctx, "test-image", mock.Anything).
		Return(io.NopCloser(strings.NewReader("")), nil).Once()

	// 1. Create
	mockClient.On("ContainerCreate", ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(container.CreateResponse{ID: "new-id"}, nil).Once()

	// 2. Start
	mockClient.On("ContainerStart", ctx, "new-id", mock.Anything).Return(nil).Once()

	// 3. Inspect
	mockClient.On("ContainerInspect", ctx, "new-id").Return(container.InspectResponse{
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"esb-net": {IPAddress: "10.0.0.2"},
			},
		},
	}, nil).Once()

	// Execute
	info, err := rt.Ensure(ctx, req)

	assert.NoError(t, err)
	assert.Equal(t, "new-id", info.ID)
	assert.Equal(t, "10.0.0.2", info.IPAddress)

	mockClient.AssertExpectations(t)
}

func TestRuntime_Ensure_AlwaysCreatesNew(t *testing.T) {
	mockClient := new(MockDockerClient)
	rt := NewRuntime(mockClient, "esb-net")

	ctx := context.Background()
	req := runtime.EnsureRequest{
		FunctionName: "test-func",
	}

	mockClient.On("ImagePull", ctx, "test-func:latest", mock.Anything).
		Return(io.NopCloser(strings.NewReader("")), nil).Once()

	// Should NOT call ContainerList for lookups anymore (if we strictly follow the factory pattern)
	// Or even if it does, it should create a NEW one.
	// To be safe and clean, let's assume it doesn't lookup.

	// 1. Create
	mockClient.On("ContainerCreate", ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(container.CreateResponse{ID: "new-id-1"}, nil).Once()

	// 2. Start
	mockClient.On("ContainerStart", ctx, "new-id-1", mock.Anything).Return(nil).Once()

	// 3. Inspect
	mockClient.On("ContainerInspect", ctx, "new-id-1").Return(container.InspectResponse{
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"esb-net": {IPAddress: "10.0.0.10"},
			},
		},
	}, nil).Once()

	// Execute first call
	info1, err := rt.Ensure(ctx, req)
	assert.NoError(t, err)
	assert.Equal(t, "new-id-1", info1.ID)

	// Execute second call - should create ANOTHER one
	mockClient.On("ImagePull", ctx, "test-func:latest", mock.Anything).
		Return(io.NopCloser(strings.NewReader("")), nil).Once()
	mockClient.On("ContainerCreate", ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(container.CreateResponse{ID: "new-id-2"}, nil).Once()
	mockClient.On("ContainerStart", ctx, "new-id-2", mock.Anything).Return(nil).Once()
	mockClient.On("ContainerInspect", ctx, "new-id-2").Return(container.InspectResponse{
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"esb-net": {IPAddress: "10.0.0.11"},
			},
		},
	}, nil).Once()

	info2, err := rt.Ensure(ctx, req)
	assert.NoError(t, err)
	assert.Equal(t, "new-id-2", info2.ID)
	assert.NotEqual(t, info1.ID, info2.ID)

	mockClient.AssertExpectations(t)
}
func TestRuntime_List(t *testing.T) {
	mockClient := new(MockDockerClient)
	rt := NewRuntime(mockClient, "esb-net")

	ctx := context.Background()

	// 1. Mock List response
	mockClient.On("ContainerList", ctx, mock.Anything).Return([]container.Summary{
		{
			ID:      "id-1",
			State:   "running",
			Created: 1000000,
			Labels:  map[string]string{runtime.LabelFunctionName: "func-1", runtime.LabelCreatedBy: runtime.ValueCreatedByAgent},
		},
		{
			ID:      "id-2",
			State:   "exited",
			Created: 2000000,
			Labels:  map[string]string{runtime.LabelFunctionName: "func-2", runtime.LabelCreatedBy: runtime.ValueCreatedByAgent},
		},
		{
			ID:     "id-3",
			Labels: map[string]string{"other": "label"}, // Should be filtered out
		},
	}, nil)

	// Execute
	states, err := rt.List(ctx)

	assert.NoError(t, err)
	assert.Len(t, states, 2)
	assert.Equal(t, "id-1", states[0].ID)
	assert.Equal(t, "func-1", states[0].FunctionName)
	assert.Equal(t, "running", states[0].Status)
	assert.Equal(t, "id-2", states[1].ID)

	mockClient.AssertExpectations(t)
}
