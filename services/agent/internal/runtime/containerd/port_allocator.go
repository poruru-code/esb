package containerd

import (
	"fmt"
	"net"
	"sync"
)

// PortAllocator manages a pool of available host ports for CNI port mapping.
type PortAllocator struct {
	mu        sync.Mutex
	min       int
	max       int
	allocated map[int]bool
}

// NewPortAllocator creates a new PortAllocator with the given port range.
func NewPortAllocator(min, max int) *PortAllocator {
	return &PortAllocator{
		min:       min,
		max:       max,
		allocated: make(map[int]bool),
	}
}

// Allocate finds and reserves an available port.
func (pa *PortAllocator) Allocate() (int, error) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	for p := pa.min; p <= pa.max; p++ {
		if !pa.allocated[p] {
			// Check if port is free in the OS
			ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
			if err != nil {
				continue // Port taken by another process
			}
			ln.Close()

			pa.allocated[p] = true
			return p, nil
		}
	}
	return 0, fmt.Errorf("no available ports in range %d-%d", pa.min, pa.max)
}

// Release returns a port to the available pool.
func (pa *PortAllocator) Release(port int) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	delete(pa.allocated, port)
}
