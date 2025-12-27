package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/docker/client"
	"github.com/poruru/edge-serverless-box/services/agent/internal/api"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime/docker"
	pb "github.com/poruru/edge-serverless-box/services/agent/pkg/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	log.Println("Starting ESB Agent...")

	// Configuration
	port := os.Getenv("PORT")
	if port == "" {
		port = "50051"
	}

	// Network Configuration
	// Gatewayや他のコンテナと同じネットワークに接続するために必要
	// default to "bridge" if not specified, but usually passed via env
	networkID := os.Getenv("CONTAINERS_NETWORK")
	if networkID == "" {
		networkID = "bridge"
		log.Println("WARNING: CONTAINERS_NETWORK not specified, defaulting to 'bridge'")
	}
	log.Printf("Target Network: %s", networkID)

	// Initialize Docker Client
	// FromEnv creates a client from environment variables (DOCKER_HOST etc.)
	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Failed to create Docker client: %v", err)
	}
	defer dockerCli.Close()

	// Verify Docker connection
	ctx := context.Background()
	info, err := dockerCli.Info(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to Docker daemon: %v", err)
	}
	log.Printf("Connected to Docker (Version: %s)", info.ServerVersion)

	// Initialize Runtime
	var rt runtime.ContainerRuntime = docker.NewRuntime(dockerCli, networkID)
	defer rt.Close()

	// Initialize gRPC Server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	agentServer := api.NewAgentServer(rt)
	pb.RegisterAgentServiceServer(grpcServer, agentServer)

	// Enable reflection for debugging (grpcurl etc.)
	reflection.Register(grpcServer)

	// Signal handling for graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		log.Println("Received shutdown signal, cleaning up...")

		// Perform GC before shutdown
		if err := rt.GC(context.Background()); err != nil {
			log.Printf("Warning: GC during shutdown failed: %v", err)
		}

		log.Println("Shutting down gRPC server...")
		grpcServer.GracefulStop()
	}()

	log.Printf("gRPC server listening on port %s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
