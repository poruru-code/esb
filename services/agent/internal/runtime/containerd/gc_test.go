package containerd

import (
	"context"
	"testing"

	"github.com/containerd/containerd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRuntime_GC(t *testing.T) {
	mockCli := new(MockClient)
	rt := NewRuntime(mockCli, nil, nil, "esb")

	// We expect GC will call Containers()
	mockCli.On("Containers", mock.Anything, mock.Anything).Return([]containerd.Container{}, nil)

	err := rt.GC(context.Background())
	assert.NoError(t, err)

	mockCli.AssertExpectations(t)
}
