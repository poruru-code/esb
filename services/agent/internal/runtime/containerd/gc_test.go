package containerd

import (
	"context"
	"errors"
	"testing"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRuntime_GC(t *testing.T) {
	mockCli := new(MockClient)
	rt := NewRuntime(mockCli, nil, "esb", "test-env")

	// We expect GC will call Containers()
	mockCli.On("Containers", mock.Anything, mock.Anything).Return([]containerd.Container{}, nil)

	err := rt.GC(context.Background())
	assert.NoError(t, err)

	mockCli.AssertExpectations(t)
}

func TestRuntime_GC_SkipsUnmanagedContainers(t *testing.T) {
	mockCli := new(MockClient)
	rt := NewRuntime(mockCli, nil, "esb", "test-env")

	managed := new(MockContainer)
	managedID := "esb-test-env-func-1234"
	managed.On("ID").Return(managedID)
	managed.On("Info", mock.Anything, mock.Anything).Return(containers.Container{
		Labels: map[string]string{
			runtime.LabelCreatedBy: runtime.ValueCreatedByAgent,
			runtime.LabelEsbEnv:    "test-env",
		},
	}, nil)
	managed.On("Task", mock.Anything, mock.Anything).Return(nil, errors.New("no task"))
	managed.On("Delete", mock.Anything, mock.Anything).Return(nil)

	unmanaged := new(MockContainer)
	unmanagedID := "esb-other-env-func-5678"
	unmanaged.On("ID").Return(unmanagedID)
	unmanaged.On("Info", mock.Anything, mock.Anything).Return(containers.Container{
		Labels: map[string]string{
			runtime.LabelCreatedBy: runtime.ValueCreatedByAgent,
			runtime.LabelEsbEnv:    "other-env",
		},
	}, nil)

	mockCli.On("Containers", mock.Anything, mock.Anything).Return(
		[]containerd.Container{managed, unmanaged},
		nil,
	)

	err := rt.GC(context.Background())
	assert.NoError(t, err)

	managed.AssertCalled(t, "Delete", mock.Anything, mock.Anything)
	unmanaged.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

func TestRuntime_GC_RemovesCNIWhenTaskExists(t *testing.T) {
	mockCli := new(MockClient)
	mockCNI := new(MockCNI)
	rt := NewRuntime(mockCli, mockCNI, "esb", "test-env")

	containerID := "esb-test-env-func-1234"
	mockContainer := new(MockContainer)
	mockContainer.On("ID").Return(containerID)
	mockContainer.On("Info", mock.Anything, mock.Anything).Return(containers.Container{
		Labels: map[string]string{
			runtime.LabelCreatedBy: runtime.ValueCreatedByAgent,
			runtime.LabelEsbEnv:    "test-env",
		},
	}, nil)

	mockTask := new(MockTask)
	mockContainer.On("Task", mock.Anything, mock.Anything).Return(mockTask, nil)
	mockTask.On("Pid").Return(uint32(1234))
	mockTask.On("Status", mock.Anything).Return(containerd.Status{Status: containerd.Stopped}, nil)
	mockTask.On("Delete", mock.Anything, mock.Anything).Return((*containerd.ExitStatus)(nil), nil)
	mockContainer.On("Delete", mock.Anything, mock.Anything).Return(nil)

	mockCNI.On(
		"Remove",
		mock.Anything,
		containerID,
		"/proc/1234/ns/net",
		mock.Anything,
	).Return(nil)

	mockCli.On("Containers", mock.Anything, mock.Anything).Return(
		[]containerd.Container{mockContainer},
		nil,
	)

	err := rt.GC(context.Background())
	assert.NoError(t, err)

	mockCNI.AssertCalled(t, "Remove", mock.Anything, containerID, "/proc/1234/ns/net", mock.Anything)
}
