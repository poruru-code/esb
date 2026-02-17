// Where: services/agent/internal/runtime/containerd/image_test.go
// What: Tests for image pulling/unpacking in containerd runtime.
// Why: Ensure snapshotter-aware image preparation for firecracker mode.
package containerd

import (
	"context"
	"fmt"
	"testing"

	"github.com/containerd/containerd/errdefs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRuntime_EnsureImage(t *testing.T) {
	mockCli := new(MockClient)
	rt := NewRuntime(mockCli, nil, "esb", "test-env", "esb")
	ctx := context.Background()
	imageName := "alpine:latest"
	t.Setenv("CONTAINERD_SNAPSHOTTER", "devmapper")

	// 1. Initial check: Image doesn't exist
	mockCli.On("GetImage", mock.Anything, imageName).Return(nil, errdefs.ErrNotFound)

	// 2. Expect Pull to be called
	mockImage := new(MockImage)
	mockImage.On("IsUnpacked", mock.Anything, "devmapper").Return(true, nil)
	mockCli.On(
		"Pull",
		mock.Anything,
		imageName,
		mock.Anything,
	).Return(mockImage, nil)

	// Since ensureImage is unexported, we might test it via a public method
	// or make it exported/internal. For TDD of internal logic,
	// we'll assume we'll have an internal method or test it via Ensure.
	// For now, let's assume we implement it as an unexported method
	// and this test will fail to compile.
	_, err := rt.ensureImage(ctx, imageName)
	assert.NoError(t, err)

	mockCli.AssertExpectations(t)
	mockImage.AssertExpectations(t)
}

func TestRuntime_EnsureImage_UnpackWhenMissing(t *testing.T) {
	mockCli := new(MockClient)
	rt := NewRuntime(mockCli, nil, "esb", "test-env", "esb")
	ctx := context.Background()
	imageName := "alpine:latest"
	t.Setenv("CONTAINERD_SNAPSHOTTER", "devmapper")

	mockImage := new(MockImage)
	mockImage.On("IsUnpacked", mock.Anything, "devmapper").Return(false, nil)
	mockImage.On("Unpack", mock.Anything, "devmapper").Return(nil)
	mockCli.On("GetImage", mock.Anything, imageName).Return(mockImage, nil)

	_, err := rt.ensureImage(ctx, imageName)
	assert.NoError(t, err)

	mockCli.AssertExpectations(t)
	mockImage.AssertExpectations(t)
}

func TestRuntime_EnsureImage_GetImageError(t *testing.T) {
	mockCli := new(MockClient)
	rt := NewRuntime(mockCli, nil, "esb", "test-env", "esb")
	ctx := context.Background()
	imageName := "alpine:latest"

	getErr := fmt.Errorf("permission denied")
	mockCli.On("GetImage", mock.Anything, imageName).Return(nil, getErr)

	_, err := rt.ensureImage(ctx, imageName)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "failed to get image")

	mockCli.AssertNumberOfCalls(t, "Pull", 0)
	mockCli.AssertExpectations(t)
}
