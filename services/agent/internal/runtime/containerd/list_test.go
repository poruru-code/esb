package containerd

import (
	"context"
	"testing"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestRuntime_List_Empty tests that List returns empty slice when no containers exist
func TestRuntime_List_Empty(t *testing.T) {
	mockCli := new(MockClient)
	mockCNI := new(MockCNI)
	rt := NewRuntime(mockCli, mockCNI, "esb")
	ctx := context.Background()

	// Mock: No containers exist
	mockCli.On("Containers", mock.Anything, mock.Anything).Return([]containerd.Container{}, nil)

	// Execute
	states, err := rt.List(ctx)

	assert.NoError(t, err)
	assert.Empty(t, states)
	mockCli.AssertExpectations(t)
}

// TestRuntime_List_ReturnsContainerStates tests that List returns correct states
func TestRuntime_List_ReturnsContainerStates(t *testing.T) {
	mockCli := new(MockClient)
	mockCNI := new(MockCNI)
	rt := NewRuntime(mockCli, mockCNI, "esb")
	ctx := context.Background()

	// Pre-populate accessTracker to simulate a container that was used
	containerID := "lambda-test-func-1234"
	testTime := time.Now().Add(-5 * time.Minute)
	rt.accessTracker.Store(containerID, testTime)

	// Mock container setup
	mockContainer := new(MockContainer)
	mockContainer.On("ID").Return(containerID)
	mockContainer.On("Info", mock.Anything, mock.Anything).Return(containers.Container{
		CreatedAt: time.Now(),
		Labels: map[string]string{
			"esb_function": "test-func",
		},
	}, nil)
	mockContainer.On("Labels", mock.Anything).Return(map[string]string{
		"esb_function": "test-func",
	}, nil).Maybe()

	mockTask := new(MockTask)
	mockContainer.On("Task", mock.Anything, mock.Anything).Return(mockTask, nil)
	mockTask.On("Status", mock.Anything).Return(containerd.Status{Status: containerd.Paused}, nil)

	// Mock: One container exists
	mockCli.On("Containers", mock.Anything, mock.Anything).Return([]containerd.Container{mockContainer}, nil)

	// Execute
	states, err := rt.List(ctx)

	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, containerID, states[0].ID)
	assert.Equal(t, "test-func", states[0].FunctionName)
	assert.Equal(t, "PAUSED", states[0].Status)
	// LastUsedAt should match what we stored
	assert.Equal(t, testTime, states[0].LastUsedAt)

	mockCli.AssertExpectations(t)
	mockContainer.AssertExpectations(t)
	mockTask.AssertExpectations(t)
}

// TestRuntime_AccessTracker_RecordsOnResume tests that Resume records access time
func TestRuntime_AccessTracker_RecordsOnResume(t *testing.T) {
	mockCli := new(MockClient)
	mockCNI := new(MockCNI)
	rt := NewRuntime(mockCli, mockCNI, "esb")
	ctx := context.Background()
	containerID := "lambda-resume-test-1234"

	mockContainer := new(MockContainer)
	mockTask := new(MockTask)

	mockCli.On("LoadContainer", mock.Anything, containerID).Return(mockContainer, nil)
	mockContainer.On("Task", mock.Anything, mock.Anything).Return(mockTask, nil)
	mockTask.On("Resume", mock.Anything).Return(nil)

	// Execute
	beforeResume := time.Now()
	err := rt.Resume(ctx, containerID)
	afterResume := time.Now()

	assert.NoError(t, err)

	// Check accessTracker was updated
	val, exists := rt.accessTracker.Load(containerID)
	assert.True(t, exists, "accessTracker should have recorded access time on Resume")
	accessTime := val.(time.Time)
	assert.True(t, accessTime.After(beforeResume) || accessTime.Equal(beforeResume))
	assert.True(t, accessTime.Before(afterResume) || accessTime.Equal(afterResume))

	mockCli.AssertExpectations(t)
	mockContainer.AssertExpectations(t)
	mockTask.AssertExpectations(t)
}
