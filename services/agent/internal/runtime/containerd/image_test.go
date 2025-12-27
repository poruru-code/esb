package containerd

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRuntime_EnsureImage(t *testing.T) {
	mockCli := new(MockClient)
	rt := NewRuntime(mockCli, nil, nil, "esb")
	ctx := context.Background()
	imageName := "alpine:latest"

	// 1. Initial check: Image doesn't exist
	mockCli.On("GetImage", mock.Anything, imageName).Return(nil, fmt.Errorf("not found"))
	
	// 2. Expect Pull to be called
	mockCli.On("Pull", mock.Anything, imageName, mock.Anything).Return(nil, nil)

	// Since ensureImage is unexported, we might test it via a public method 
	// or make it exported/internal. For TDD of internal logic, 
	// we'll assume we'll have an internal method or test it via Ensure.
	// For now, let's assume we implement it as an unexported method 
	// and this test will fail to compile.
	_, err := rt.ensureImage(ctx, imageName)
	assert.NoError(t, err)
	
	mockCli.AssertExpectations(t)
}
