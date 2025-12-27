package api

import (
	"context"

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

	if err := s.runtime.Pause(ctx, req.ContainerId); err != nil {
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
		})
	}

	return &pb.ListContainersResponse{
		Containers: containers,
	}, nil
}
