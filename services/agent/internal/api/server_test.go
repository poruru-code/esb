package api_test

import (
	"context"
	"net"
	"testing"

	"github.com/poruru/edge-serverless-box/services/agent/internal/api"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
	pb "github.com/poruru/edge-serverless-box/services/agent/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// MockRuntime mocks the container runtime
type MockRuntime struct {
	mock.Mock
}

// Ensure interface match. Note: Since we haven't defined runtime interface yet in api package,
// we will assume a structural interface matching what calls.
// The real runtime.WorkerInfo is in internal/runtime.
// We should import runtime package to use its types.
// But circular dependency check: api imports runtime. runtime does not import api. Safe.

// For now, let's define the interface in the test or use the concrete type from runtime package.
// We'll update imports to include runtime package.

func (m *MockRuntime) EnsureContainer(ctx context.Context, functionName string, image string, env map[string]string) (*runtime.WorkerInfo, error) {
	args := m.Called(ctx, functionName, image, env)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*runtime.WorkerInfo), args.Error(1)
}

func (m *MockRuntime) DestroyContainer(ctx context.Context, containerID string) error {
	args := m.Called(ctx, containerID)
	return args.Error(0)
}

const bufSize = 1024 * 1024

var lis *bufconn.Listener

func initServer(t *testing.T, mockRT *MockRuntime) *grpc.ClientConn {
	lis = bufconn.Listen(bufSize)
	s := grpc.NewServer()

	// Inject mock runtime
	server := api.NewAgentServer(mockRT)
	pb.RegisterAgentServiceServer(s, server)

	go func() {
		if err := s.Serve(lis); err != nil {
			// Fail test if server fails
		}
	}()

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}
	return conn
}

func TestEnsureContainer(t *testing.T) {
	mockRT := new(MockRuntime)
	conn := initServer(t, mockRT)
	defer conn.Close()

	client := pb.NewAgentServiceClient(conn)

	fnName := "test-func"
	image := "test-image"
	env := map[string]string{"foo": "bar"}

	expectedWorker := &runtime.WorkerInfo{
		ID:        "container-123",
		Name:      "lambda-test-func-123",
		IPAddress: "10.0.0.9",
		Port:      8080,
	}

	mockRT.On("EnsureContainer", mock.Anything, fnName, image, env).Return(expectedWorker, nil)

	req := &pb.EnsureContainerRequest{
		FunctionName: fnName,
		Image:        image,
		Env:          env,
	}

	resp, err := client.EnsureContainer(context.Background(), req)

	assert.NoError(t, err)
	assert.Equal(t, expectedWorker.ID, resp.Id)
	assert.Equal(t, expectedWorker.IPAddress, resp.IpAddress)

	mockRT.AssertExpectations(t)
}
