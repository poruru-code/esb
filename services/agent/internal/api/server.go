package api

import (
	"context"

	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
	pb "github.com/poruru/edge-serverless-box/services/agent/pkg/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ContainerRuntime defines the interface for underlying container operations.
// This decouples the gRPC layer from the specific runtime implementation (Docker, containerd).
type ContainerRuntime interface {
	EnsureContainer(ctx context.Context, functionName string, image string, env map[string]string) (*runtime.WorkerInfo, error)
	DestroyContainer(ctx context.Context, containerID string) error
}

type AgentServer struct {
	pb.UnimplementedAgentServiceServer
	runtime ContainerRuntime
}

func NewAgentServer(rt ContainerRuntime) *AgentServer {
	return &AgentServer{
		runtime: rt,
	}
}

func (s *AgentServer) EnsureContainer(ctx context.Context, req *pb.EnsureContainerRequest) (*pb.WorkerInfo, error) {
	if req.FunctionName == "" {
		return nil, status.Error(codes.InvalidArgument, "function_name is required")
	}

	info, err := s.runtime.EnsureContainer(ctx, req.FunctionName, req.Image, req.Env)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to ensure container: %v", err)
	}

	return &pb.WorkerInfo{
		Id:        info.ID,
		Name:      info.Name,
		IpAddress: info.IPAddress,
		Port:      info.Port,
	}, nil
}

func (s *AgentServer) DestroyContainer(ctx context.Context, req *pb.DestroyContainerRequest) (*pb.DestroyContainerResponse, error) {
	if req.ContainerId == "" {
		return nil, status.Error(codes.InvalidArgument, "container_id is required")
	}

	if err := s.runtime.DestroyContainer(ctx, req.ContainerId); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to destroy container: %v", err)
	}

	return &pb.DestroyContainerResponse{
		Success: true,
	}, nil
}
