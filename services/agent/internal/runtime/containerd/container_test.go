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
