package containerd

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/go-cni"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRuntime_Ensure_NewContainer(t *testing.T) {
	mockCli := new(MockClient)
	mockCNI := new(MockCNI)
	mockPA := NewPortAllocator(20000, 20000)
	rt := NewRuntime(mockCli, mockCNI, mockPA, "esb")
	ctx := context.Background()
	req := runtime.EnsureRequest{
		FunctionName: "test-func",
		Image:        "alpine:latest",
	}

	// 1. EnsureImage
	mockImage := new(MockImage)
	mockCli.On("GetImage", mock.Anything, "alpine:latest").Return(mockImage, nil)

	// 2. Containers check
	mockCli.On("Containers", mock.Anything, mock.Anything).Return([]containerd.Container{}, nil)

	// 3. NewContainer
	mockContainer := new(MockContainer)
	mockContainer.On("ID").Return("lambda-test-func-1234") // Match runtime logic
	mockCli.On("NewContainer", mock.Anything, "lambda-test-func-1234", mock.Anything).Return(mockContainer, nil)

	// 4. NewTask & Start
	mockTask := new(MockTask)
	mockTask.On("Pid").Return(uint32(1234))

	// Verify IO creator is passed (Log configuration check)
	ioCreatorMatcher := mock.MatchedBy(func(c cio.Creator) bool {
		return c != nil
	})
	mockContainer.On("NewTask", mock.Anything, ioCreatorMatcher, mock.Anything).Return(mockTask, nil)

	mockTask.On("Start", mock.Anything).Return(nil)

	// 5. CNI Setup (Expected)
	res := &cni.Result{
		Interfaces: map[string]*cni.Config{
			"eth0": {
				IPConfigs: []*cni.IPConfig{
					{IP: net.ParseIP("10.88.0.2")},
				},
			},
		},
	}
	// Setup is called with context, id, namespace path, and options (PortMap)
	// We use mock.Anything for arguments to avoid strict matching of path/opts
	mockCNI.On("Setup", mock.Anything, "lambda-test-func-1234", "/proc/1234/ns/net", mock.Anything).Return(res, nil)

	// Execute
	info, err := rt.Ensure(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.Equal(t, "lambda-test-func-1234", info.ID)
	assert.Equal(t, "10.88.0.2", info.IPAddress)
	assert.Equal(t, 20000, info.Port)

	mockCli.AssertExpectations(t)
	mockCNI.AssertExpectations(t)
	mockContainer.AssertExpectations(t)
	mockTask.AssertExpectations(t)
}

func TestRuntime_Ensure_NetworkFailure_Rollback(t *testing.T) {
	mockCli := new(MockClient)
	mockCNI := new(MockCNI)
	mockPA := NewPortAllocator(20000, 20000)
	rt := NewRuntime(mockCli, mockCNI, mockPA, "esb")
	ctx := context.Background()
	req := runtime.EnsureRequest{
		FunctionName: "rollback-func",
		Image:        "alpine:latest",
	}

	// 1. EnsureImage
	mockImage := new(MockImage)
	mockCli.On("GetImage", mock.Anything, "alpine:latest").Return(mockImage, nil)

	// 2. Containers check
	mockCli.On("Containers", mock.Anything, mock.Anything).Return([]containerd.Container{}, nil)

	// 3. NewContainer
	mockContainer := new(MockContainer)
	mockContainer.On("ID").Return("lambda-rollback-func-1234")
	mockCli.On("NewContainer", mock.Anything, "lambda-rollback-func-1234", mock.Anything).Return(mockContainer, nil)

	// 4. NewTask & Start
	mockTask := new(MockTask)
	mockTask.On("Pid").Return(uint32(5678))
	mockContainer.On("NewTask", mock.Anything, mock.Anything, mock.Anything).Return(mockTask, nil)
	mockTask.On("Start", mock.Anything).Return(nil)

	// 5. CNI Setup Failure
	// Setup fails, triggering rollback
	mockCNI.On("Setup", mock.Anything, "lambda-rollback-func-1234", "/proc/5678/ns/net", mock.Anything).Return(nil, fmt.Errorf("cni error"))

	// 6. Rollback Expectations (Context separated cleanup)
	// Verify that the context passed to cleanup has a deadline (Timeout context)
	// identifying it as the detached context, not the original background context.
	ctxWithDeadlineMatcher := mock.MatchedBy(func(c context.Context) bool {
		_, ok := c.Deadline()
		return ok
	})

	// Expect Delete on Task (ProcessKill) with detached context
	mockTask.On("Delete", ctxWithDeadlineMatcher, mock.Anything).Return(nil, nil)
	// Expect Delete on Container (SnapshotCleanup) with detached context
	mockContainer.On("Delete", ctxWithDeadlineMatcher, mock.Anything).Return(nil)

	// Execute
	info, err := rt.Ensure(ctx, req)

	assert.Error(t, err)
	assert.Nil(t, info)
	assert.Contains(t, err.Error(), "cni error")

	mockCli.AssertExpectations(t)
	mockCNI.AssertExpectations(t)
	mockContainer.AssertExpectations(t)
	mockTask.AssertExpectations(t)
}

// Red Test: Pause should load the container and pause the task
func TestRuntime_Pause_Red(t *testing.T) {
	mockCli := new(MockClient)
	mockCNI := new(MockCNI)
	mockPA := NewPortAllocator(20000, 20100)
	rt := NewRuntime(mockCli, mockCNI, mockPA, "esb")
	ctx := context.Background()
	containerID := "lambda-test-func-1234"

	mockContainer := new(MockContainer)
	mockTask := new(MockTask)

	// Expect LoadContainer to be called
	mockCli.On("LoadContainer", mock.Anything, containerID).Return(mockContainer, nil)
	// Expect Task to be called to get the existing task
	mockContainer.On("Task", mock.Anything, mock.Anything).Return(mockTask, nil)
	// Expect Pause on task
	mockTask.On("Pause", mock.Anything).Return(nil)

	err := rt.Pause(ctx, containerID)

	assert.NoError(t, err)
	mockCli.AssertExpectations(t)
	mockContainer.AssertExpectations(t)
	mockTask.AssertExpectations(t)
}

// Red Test: Resume should load the container and resume the task
func TestRuntime_Resume_Red(t *testing.T) {
	mockCli := new(MockClient)
	mockCNI := new(MockCNI)
	mockPA := NewPortAllocator(20000, 20100)
	rt := NewRuntime(mockCli, mockCNI, mockPA, "esb")
	ctx := context.Background()
	containerID := "lambda-test-func-1234"

	mockContainer := new(MockContainer)
	mockTask := new(MockTask)

	// Expect LoadContainer to be called
	mockCli.On("LoadContainer", mock.Anything, containerID).Return(mockContainer, nil)
	// Expect Task to be called to get the existing task
	mockContainer.On("Task", mock.Anything, mock.Anything).Return(mockTask, nil)
	// Expect Resume on task
	mockTask.On("Resume", mock.Anything).Return(nil)

	err := rt.Resume(ctx, containerID)

	assert.NoError(t, err)
	mockCli.AssertExpectations(t)
	mockContainer.AssertExpectations(t)
	mockTask.AssertExpectations(t)
}

// Red Test: Warm Start - Ensure should detect Paused container and Resume it
func TestRuntime_Ensure_WarmStart_Paused_Red(t *testing.T) {
	mockCli := new(MockClient)
	mockCNI := new(MockCNI)
	mockPA := NewPortAllocator(20000, 20100)
	rt := NewRuntime(mockCli, mockCNI, mockPA, "esb")
	ctx := context.Background()
	req := runtime.EnsureRequest{
		FunctionName: "warm-func",
		Image:        "alpine:latest",
	}

	// Mock existing container
	mockContainer := new(MockContainer)
	mockContainer.On("ID").Return("lambda-warm-func-1234")
	mockCli.On("Containers", mock.Anything, mock.Anything).Return([]containerd.Container{mockContainer}, nil)

	// Mock task status check
	mockTask := new(MockTask)
	mockContainer.On("Task", mock.Anything, mock.Anything).Return(mockTask, nil)
	mockTask.On("Status", mock.Anything).Return(containerd.Status{Status: containerd.Paused}, nil)

	// Expect Resume to be called (Warm Start)
	mockTask.On("Resume", mock.Anything).Return(nil)

	// Execute
	info, err := rt.Ensure(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.Equal(t, "lambda-warm-func-1234", info.ID)
	// IP/Port should be retained (how to get them without CNI call?)
	// For now, we just check the container ID

	mockCli.AssertExpectations(t)
	mockContainer.AssertExpectations(t)
	mockTask.AssertExpectations(t)
}
