package containerd

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestPortAllocator_Allocate(t *testing.T) {
	min, max := 20000, 20005
	pa := NewPortAllocator(min, max)

	// Allocate all available ports
	allocated := make(map[int]bool)
	for i := 0; i < (max - min + 1); i++ {
		port, err := pa.Allocate()
		assert.NoError(t, err)
		assert.True(t, port >= min && port <= max)
		assert.False(t, allocated[port], "Port %d was already allocated", port)
		allocated[port] = true
	}

	// Next allocation should fail (pool exhausted)
	_, err := pa.Allocate()
	assert.Error(t, err)
}

func TestPortAllocator_Release(t *testing.T) {
	pa := NewPortAllocator(20000, 20000)
	
	port, err := pa.Allocate()
	assert.NoError(t, err)
	assert.Equal(t, 20000, port)

	// Exhausted
	_, err = pa.Allocate()
	assert.Error(t, err)

	// Release and re-allocate
	pa.Release(port)
	port2, err := pa.Allocate()
	assert.NoError(t, err)
	assert.Equal(t, port, port2)
}
