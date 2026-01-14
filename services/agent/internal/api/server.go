package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
	pb "github.com/poruru/edge-serverless-box/services/agent/pkg/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AgentServer struct {
	pb.UnimplementedAgentServiceServer
	runtime runtime.ContainerRuntime
}

func NewAgentServer(rt runtime.ContainerRuntime) *AgentServer {
	return &AgentServer{
		runtime: rt,
	}
}

func (s *AgentServer) EnsureContainer(ctx context.Context, req *pb.EnsureContainerRequest) (*pb.WorkerInfo, error) {
	if req.FunctionName == "" {
		return nil, status.Error(codes.InvalidArgument, "function_name is required")
	}

	info, err := s.runtime.Ensure(ctx, runtime.EnsureRequest{
		FunctionName: req.FunctionName,
		Image:        req.Image,
		Env:          req.Env,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to ensure container: %v", err)
	}

	return &pb.WorkerInfo{
		Id:        info.ID,
		IpAddress: info.IPAddress,
		Port:      int32(info.Port),
	}, nil
}

func (s *AgentServer) InvokeWorker(ctx context.Context, req *pb.InvokeWorkerRequest) (*pb.InvokeWorkerResponse, error) {
	if req.IpAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "ip_address is required")
	}

	port := req.Port
	if port == 0 {
		port = 8080
	}

	path := req.Path
	if path == "" {
		path = "/2015-03-31/functions/function/invocations"
	}
	if path[0] != '/' {
		path = "/" + path
	}

	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	if req.TimeoutMs <= 0 {
		timeout = 30 * time.Second
	}

	url := fmt.Sprintf("http://%s:%d%s", req.IpAddress, port, path)

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	connectTimeout := 2 * time.Second
	if timeout > 0 && timeout < connectTimeout {
		connectTimeout = timeout
	}
	transport := &http.Transport{
		DialContext: (&net.Dialer{Timeout: connectTimeout}).DialContext,
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
	backoff := 100 * time.Millisecond
	for {
		httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(req.Payload))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to build request: %v", err)
		}
		for key, value := range req.Headers {
			if key == "" {
				continue
			}
			httpReq.Header.Set(key, value)
		}

		resp, err := client.Do(httpReq)
		if err == nil {
			defer resp.Body.Close()
			body, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				return nil, status.Errorf(codes.Internal, "failed to read response body: %v", readErr)
			}

			headers := make(map[string]string, len(resp.Header))
			for key, values := range resp.Header {
				if len(values) > 0 {
					headers[key] = values[0]
				}
			}

			return &pb.InvokeWorkerResponse{
				StatusCode: int32(resp.StatusCode),
				Headers:    headers,
				Body:       body,
			}, nil
		}

		if reqCtx.Err() != nil {
			return nil, status.Errorf(codes.DeadlineExceeded, "invoke timeout: %v", err)
		}

		time.Sleep(backoff)
		if backoff < time.Second {
			backoff *= 2
			if backoff > time.Second {
				backoff = time.Second
			}
		}
	}
}

func (s *AgentServer) DestroyContainer(ctx context.Context, req *pb.DestroyContainerRequest) (*pb.DestroyContainerResponse, error) {
	if req.ContainerId == "" {
		return nil, status.Error(codes.InvalidArgument, "container_id is required")
	}

	if err := s.runtime.Destroy(ctx, req.ContainerId); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to destroy container: %v", err)
	}

	return &pb.DestroyContainerResponse{
		Success: true,
	}, nil
}

func (s *AgentServer) PauseContainer(ctx context.Context, req *pb.PauseContainerRequest) (*pb.PauseContainerResponse, error) {
	if req.ContainerId == "" {
		return nil, status.Error(codes.InvalidArgument, "container_id is required")
	}

	if err := s.runtime.Suspend(ctx, req.ContainerId); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to pause container: %v", err)
	}

	return &pb.PauseContainerResponse{
		Success: true,
	}, nil
}

func (s *AgentServer) ResumeContainer(ctx context.Context, req *pb.ResumeContainerRequest) (*pb.ResumeContainerResponse, error) {
	if req.ContainerId == "" {
		return nil, status.Error(codes.InvalidArgument, "container_id is required")
	}

	if err := s.runtime.Resume(ctx, req.ContainerId); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to resume container: %v", err)
	}

	return &pb.ResumeContainerResponse{
		Success: true,
	}, nil
}

func (s *AgentServer) ListContainers(ctx context.Context, req *pb.ListContainersRequest) (*pb.ListContainersResponse, error) {
	states, err := s.runtime.List(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list containers: %v", err)
	}

	var containers []*pb.ContainerState
	for _, s := range states {
		containers = append(containers, &pb.ContainerState{
			ContainerId:   s.ID,
			FunctionName:  s.FunctionName,
			Status:        s.Status,
			LastUsedAt:    s.LastUsedAt.Unix(),
			ContainerName: s.ContainerName,
			CreatedAt:     s.CreatedAt.Unix(),
		})
	}

	return &pb.ListContainersResponse{
		Containers: containers,
	}, nil
}

func (s *AgentServer) GetContainerMetrics(ctx context.Context, req *pb.GetContainerMetricsRequest) (*pb.GetContainerMetricsResponse, error) {
	if req.ContainerId == "" {
		return nil, status.Error(codes.InvalidArgument, "container_id is required")
	}

	metrics, err := s.runtime.Metrics(ctx, req.ContainerId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get container metrics: %v", err)
	}

	return &pb.GetContainerMetricsResponse{
		Metrics: &pb.ContainerMetrics{
			ContainerId:   metrics.ID,
			FunctionName:  metrics.FunctionName,
			ContainerName: metrics.ContainerName,
			State:         metrics.State,
			MemoryCurrent: metrics.MemoryCurrent,
			MemoryMax:     metrics.MemoryMax,
			OomEvents:     metrics.OOMEvents,
			CpuUsageNs:    metrics.CPUUsageNS,
			ExitCode:      metrics.ExitCode,
			RestartCount:  metrics.RestartCount,
			ExitTime:      toUnixSeconds(metrics.ExitTime),
			CollectedAt:   toUnixSeconds(metrics.CollectedAt),
		},
	}, nil
}

func toUnixSeconds(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.Unix()
}
