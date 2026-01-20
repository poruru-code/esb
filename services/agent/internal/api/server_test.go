package api_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/containerd/containerd/errdefs"
	"github.com/poruru/edge-serverless-box/services/agent/internal/api"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
	pb "github.com/poruru/edge-serverless-box/services/agent/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
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

func (m *MockRuntime) Ensure(ctx context.Context, req runtime.EnsureRequest) (*runtime.WorkerInfo, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*runtime.WorkerInfo), args.Error(1)
}

func (m *MockRuntime) Destroy(ctx context.Context, containerID string) error {
	args := m.Called(ctx, containerID)
	return args.Error(0)
}

func (m *MockRuntime) Suspend(ctx context.Context, containerID string) error {
	args := m.Called(ctx, containerID)
	return args.Error(0)
}

func (m *MockRuntime) Resume(ctx context.Context, containerID string) error {
	args := m.Called(ctx, containerID)
	return args.Error(0)
}

func (m *MockRuntime) List(ctx context.Context) ([]runtime.ContainerState, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]runtime.ContainerState), args.Error(1)
}

func (m *MockRuntime) Metrics(ctx context.Context, containerID string) (*runtime.ContainerMetrics, error) {
	args := m.Called(ctx, containerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*runtime.ContainerMetrics), args.Error(1)
}

func (m *MockRuntime) Touch(id string) {
	m.Called(id)
}

func (m *MockRuntime) GC(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockRuntime) Close() error {
	args := m.Called()
	return args.Error(0)
}

const bufSize = 1024 * 1024

func initServer(t *testing.T, mockRT *MockRuntime) *grpc.ClientConn {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	s := grpc.NewServer()

	// Inject mock runtime
	server := api.NewAgentServer(mockRT)
	pb.RegisterAgentServiceServer(s, server)

	go func() {
		_ = s.Serve(lis)
	}()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		s.Stop()
		_ = lis.Close()
	})
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
		IPAddress: "10.0.0.9",
		Port:      8080,
	}

	mockRT.On("Ensure", mock.Anything, runtime.EnsureRequest{
		FunctionName: fnName,
		Image:        image,
		Env:          env,
	}).Return(expectedWorker, nil)

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

func TestDestroyContainer(t *testing.T) {
	mockRT := new(MockRuntime)
	conn := initServer(t, mockRT)
	defer conn.Close()

	client := pb.NewAgentServiceClient(conn)
	containerID := "test-container-id"

	mockRT.On("Destroy", mock.Anything, containerID).Return(nil)

	req := &pb.DestroyContainerRequest{
		ContainerId: containerID,
	}

	resp, err := client.DestroyContainer(context.Background(), req)

	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockRT.AssertExpectations(t)
}

func TestDestroyContainerNotFound(t *testing.T) {
	mockRT := new(MockRuntime)
	conn := initServer(t, mockRT)
	defer conn.Close()

	client := pb.NewAgentServiceClient(conn)
	containerID := "missing-container-id"

	mockRT.On("Destroy", mock.Anything, containerID).Return(errdefs.ErrNotFound)

	req := &pb.DestroyContainerRequest{
		ContainerId: containerID,
	}

	resp, err := client.DestroyContainer(context.Background(), req)

	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockRT.AssertExpectations(t)
}

func TestInvokeWorkerRequiresContainerID(t *testing.T) {
	mockRT := new(MockRuntime)
	conn := initServer(t, mockRT)
	defer conn.Close()

	client := pb.NewAgentServiceClient(conn)

	_, err := client.InvokeWorker(context.Background(), &pb.InvokeWorkerRequest{})
	assert.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestInvokeWorkerUsesCachedWorker(t *testing.T) {
	mockRT := new(MockRuntime)
	conn := initServer(t, mockRT)
	defer conn.Close()

	client := pb.NewAgentServiceClient(conn)

	var gotPath string
	var gotHeader string
	var gotBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHeader = r.Header.Get("X-Test")
		body, err := io.ReadAll(r.Body)
		if err == nil {
			gotBody = string(body)
		}
		w.Header().Set("X-From", "worker")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	host, portStr, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	fnName := "test-func"
	image := "test-image"
	var env map[string]string
	expectedWorker := &runtime.WorkerInfo{
		ID:        "container-123",
		IPAddress: host,
		Port:      port,
	}

	mockRT.On("Ensure", mock.Anything, runtime.EnsureRequest{
		FunctionName: fnName,
		Image:        image,
		Env:          env,
	}).Return(expectedWorker, nil)

	mockRT.On("Touch", expectedWorker.ID).Return()

	_, err = client.EnsureContainer(context.Background(), &pb.EnsureContainerRequest{
		FunctionName: fnName,
		Image:        image,
		Env:          env,
	})
	assert.NoError(t, err)

	resp, err := client.InvokeWorker(context.Background(), &pb.InvokeWorkerRequest{
		ContainerId: expectedWorker.ID,
		Path:        "invoke",
		Payload:     []byte("hello"),
		Headers: map[string]string{
			"X-Test": "value",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, int32(http.StatusAccepted), resp.StatusCode)
	assert.Equal(t, "ok", string(resp.Body))
	assert.Equal(t, "worker", resp.Headers["X-From"])
	assert.Equal(t, "/invoke", gotPath)
	assert.Equal(t, "value", gotHeader)
	assert.Equal(t, "hello", gotBody)
}

func TestInvokeWorkerRefreshesCacheOnMiss(t *testing.T) {
	mockRT := new(MockRuntime)
	conn := initServer(t, mockRT)
	defer conn.Close()

	client := pb.NewAgentServiceClient(conn)

	var gotPath string
	var gotHeader string
	var gotBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHeader = r.Header.Get("X-Test")
		body, err := io.ReadAll(r.Body)
		if err == nil {
			gotBody = string(body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	host, portStr, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	containerID := "container-123"
	mockRT.On("List", mock.Anything).Return([]runtime.ContainerState{
		{
			ID:        containerID,
			IPAddress: host,
			Port:      port,
		},
	}, nil)

	mockRT.On("Touch", containerID).Return()

	resp, err := client.InvokeWorker(context.Background(), &pb.InvokeWorkerRequest{
		ContainerId: containerID,
		Path:        "invoke",
		Payload:     []byte("hello"),
		Headers: map[string]string{
			"X-Test": "value",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, int32(http.StatusOK), resp.StatusCode)
	assert.Equal(t, "ok", string(resp.Body))
	assert.Equal(t, "/invoke", gotPath)
	assert.Equal(t, "value", gotHeader)
	assert.Equal(t, "hello", gotBody)

	mockRT.AssertExpectations(t)
}

func TestPauseContainer(t *testing.T) {
	mockRT := new(MockRuntime)
	conn := initServer(t, mockRT)
	defer conn.Close()

	client := pb.NewAgentServiceClient(conn)
	containerID := "test-container-id"

	mockRT.On("Suspend", mock.Anything, containerID).Return(nil)

	req := &pb.PauseContainerRequest{
		ContainerId: containerID,
	}

	resp, err := client.PauseContainer(context.Background(), req)

	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockRT.AssertExpectations(t)
}

func TestResumeContainer(t *testing.T) {
	mockRT := new(MockRuntime)
	conn := initServer(t, mockRT)
	defer conn.Close()

	client := pb.NewAgentServiceClient(conn)
	containerID := "test-container-id"

	mockRT.On("Resume", mock.Anything, containerID).Return(nil)

	req := &pb.ResumeContainerRequest{
		ContainerId: containerID,
	}

	resp, err := client.ResumeContainer(context.Background(), req)

	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockRT.AssertExpectations(t)
}

func TestGetContainerMetrics(t *testing.T) {
	mockRT := new(MockRuntime)
	conn := initServer(t, mockRT)
	defer conn.Close()

	client := pb.NewAgentServiceClient(conn)
	containerID := "test-container-id"
	now := time.Now()

	expectedMetrics := &runtime.ContainerMetrics{
		ID:            containerID,
		FunctionName:  "test-func",
		ContainerName: "lambda-test-func-123",
		State:         "RUNNING",
		MemoryCurrent: 1024,
		MemoryMax:     2048,
		OOMEvents:     1,
		CPUUsageNS:    999,
		ExitCode:      0,
		RestartCount:  0,
		ExitTime:      now,
		CollectedAt:   now,
	}

	mockRT.On("Metrics", mock.Anything, containerID).Return(expectedMetrics, nil)

	req := &pb.GetContainerMetricsRequest{
		ContainerId: containerID,
	}

	resp, err := client.GetContainerMetrics(context.Background(), req)

	assert.NoError(t, err)
	assert.NotNil(t, resp.Metrics)
	assert.Equal(t, containerID, resp.Metrics.ContainerId)
	assert.Equal(t, expectedMetrics.FunctionName, resp.Metrics.FunctionName)
	assert.Equal(t, expectedMetrics.ContainerName, resp.Metrics.ContainerName)
	assert.Equal(t, expectedMetrics.State, resp.Metrics.State)
	assert.Equal(t, expectedMetrics.MemoryCurrent, resp.Metrics.MemoryCurrent)
	assert.Equal(t, expectedMetrics.MemoryMax, resp.Metrics.MemoryMax)
	assert.Equal(t, expectedMetrics.OOMEvents, resp.Metrics.OomEvents)
	assert.Equal(t, expectedMetrics.CPUUsageNS, resp.Metrics.CpuUsageNs)
	assert.Equal(t, expectedMetrics.ExitCode, resp.Metrics.ExitCode)
	assert.Equal(t, expectedMetrics.RestartCount, resp.Metrics.RestartCount)
	assert.Equal(t, expectedMetrics.ExitTime.Unix(), resp.Metrics.ExitTime)
	assert.Equal(t, expectedMetrics.CollectedAt.Unix(), resp.Metrics.CollectedAt)

	mockRT.AssertExpectations(t)
}
