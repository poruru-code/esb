package containerd

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/containerd/go-cni"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRuntime_SetupNetwork(t *testing.T) {
	mockCNI := new(MockCNI)
	mockPA := NewPortAllocator(20000, 20000)
	
	// Create Runtime with mocks
	rt := NewRuntime(nil, mockCNI, mockPA, "esb")

	mockC := new(MockContainer)
	mockC.On("ID").Return("test-container")

	mockT := new(MockTask)
	mockT.On("Pid").Return(uint32(1234))

	ctx := context.Background()

	// Mock successful CNI setup
	// Note: go-cni types can be tricky. Using the types as defined in go-cni.
	res := &cni.Result{
		Interfaces: map[string]*cni.Config{
			"eth0": {
				IPConfigs: []*cni.IPConfig{
					{IP: net.ParseIP("10.88.0.2")},
				},
			},
		},
	}
	mockCNI.On("Setup", ctx, "test-container", "/proc/1234/ns/net", mock.Anything).Return(res, nil)

	// Test Setup
	// setupNetwork is unexported. We are in the same package.
	ip, port, err := rt.setupNetwork(ctx, mockC, mockT)

	assert.NoError(t, err)
	assert.Equal(t, "10.88.0.2", ip)
	assert.Equal(t, 20000, port)
	
	mockCNI.AssertExpectations(t)
}

func TestRuntime_SetupNetwork_RetryRollback(t *testing.T) {
	mockCNI := new(MockCNI)
	mockPA := NewPortAllocator(20000, 20000)
	
	rt := NewRuntime(nil, mockCNI, mockPA, "esb")

	mockC := new(MockContainer)
	mockC.On("ID").Return("test-container")
	mockT := new(MockTask)
	mockT.On("Pid").Return(uint32(1234))

	ctx := context.Background()

	// 1. Mock CNI setup failure (e.g., bridge conflict)
	mockCNI.On("Setup", ctx, "test-container", "/proc/1234/ns/net", mock.Anything).Return(nil, fmt.Errorf("cni error"))

	// Test Setup failure
	_, _, err := rt.setupNetwork(ctx, mockC, mockT)
	assert.Error(t, err)
	
	// Verify port was released (can allocate again)
	port2, err := mockPA.Allocate()
	assert.NoError(t, err)
	assert.Equal(t, 20000, port2)
}
