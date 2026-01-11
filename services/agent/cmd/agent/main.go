// Where: services/agent/cmd/agent/main.go
// What: Bootstrap the ESB Agent runtime and CNI setup.
// Why: Keep startup wiring and env-driven networking behavior centralized.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/go-cni"
	"github.com/docker/docker/client"
	"github.com/poruru/edge-serverless-box/services/agent/internal/api"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
	agentContainerd "github.com/poruru/edge-serverless-box/services/agent/internal/runtime/containerd"
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
	networkID := os.Getenv("CONTAINERS_NETWORK")
	if networkID == "" {
		networkID = "bridge"
		log.Println("WARNING: CONTAINERS_NETWORK not specified, defaulting to 'bridge'")
	}
	log.Printf("Target Network: %s", networkID)

	// Phase 7: Environment Isolation
	esbEnv := os.Getenv("ESB_ENV")
	if esbEnv == "" {
		esbEnv = "default"
	}
	log.Printf("ESB Environment: %s", esbEnv)

	// Initialize Runtime
	var rt runtime.ContainerRuntime

	runtimeType := os.Getenv("AGENT_RUNTIME")
	if runtimeType == "containerd" {
		log.Println("Initializing containerd Runtime...")

		// 1. Initialize containerd client
		// Assumes /run/containerd/containerd.sock is mounted
		c, err := containerd.New("/run/containerd/containerd.sock")
		if err != nil {
			log.Fatalf("Failed to create containerd client: %v", err)
		}
		// Clean up containerd client on exit
		// Note: rt.Close() currently calls client.Close() so we might explicitly handle it via rt
		// But verify if wrapper handles it.

		wrappedClient := &agentContainerd.ClientWrapper{Client: c}

		cniConfDir := os.Getenv("CNI_CONF_DIR")
		if cniConfDir == "" {
			cniConfDir = "/etc/cni/net.d"
		}

		cniConfFile := os.Getenv("CNI_CONF_FILE")
		if cniConfFile == "" {
			cniConfFile = fmt.Sprintf("%s/10-esb.conflist", cniConfDir)
		}

		cniSubnet := strings.TrimSpace(os.Getenv("CNI_SUBNET"))
		if cniSubnet != "" {
			overriddenConfFile, err := prepareCNIConfig(cniConfFile, cniSubnet)
			if err != nil {
				log.Printf("WARN: failed to apply CNI_SUBNET=%s: %v", cniSubnet, err)
			} else {
				cniConfFile = overriddenConfFile
				cniConfDir = filepath.Dir(overriddenConfFile)
				log.Printf("CNI subnet override applied: %s", cniSubnet)
			}
		}

		cniBinDir := os.Getenv("CNI_BIN_DIR")
		if cniBinDir == "" {
			cniBinDir = "/opt/cni/bin"
		}

		cniPlugin, err := cni.New(
			cni.WithPluginConfDir(cniConfDir),
			cni.WithPluginDir([]string{cniBinDir}),
			cni.WithInterfacePrefix("eth"),
			cni.WithMinNetworkCount(1),
		)
		if err != nil {
			log.Fatalf("Failed to initialize CNI: %v", err)
		}

		if err := cniPlugin.Load(cni.WithConfListFile(cniConfFile)); err != nil {
			log.Fatalf("Failed to load CNI config %s: %v", cniConfFile, err)
		}

		// 2. Create Runtime with CNI networking
		// Phase 7: Use stable namespace to match cgroup delegation.
		namespace := "esb"
		rt = agentContainerd.NewRuntime(wrappedClient, cniPlugin, namespace, esbEnv)
		log.Printf("Runtime: containerd (initialized with CNI, namespace=%s)", namespace)

	} else {
		log.Println("Initializing Docker Runtime...")

		// Initialize Docker Client
		dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			log.Fatalf("Failed to create Docker client: %v", err)
		}
		// Docker client close is handled by rt.Close() or manually here?
		// docker.NewRuntime takes *client.Client.
		// runtime.docker.Runtime implementation of Close() should close the client?
		// Looking at usage in previous main.go, it deferred dockerCli.Close().
		// Let's keep it safe.
		// However, if rt.Close() closes it, double close might be issue or harmless.
		// Check docker runtime implementation if possible, or just don't close here and let rt handle it.
		// For now, let's assume we pass ownership or rt doesn't close it.
		// In previous code: defer dockerCli.Close() and defer rt.Close().
		// So we should close dockerCli?
		// But rt is interface.

		// To be cleaner:
		// To be cleaner:
		rt = docker.NewRuntime(dockerCli, networkID, esbEnv)
		log.Println("Runtime: docker (initialized)")

		// Note: previous code verified docker connection here.
		ctx := context.Background()
		info, err := dockerCli.Info(ctx)
		if err != nil {
			log.Fatalf("Failed to connect to Docker daemon: %v", err)
		}
		log.Printf("Connected to Docker (Version: %s)", info.ServerVersion)
	}

	defer func() {
		if rt != nil {
			rt.Close()
		}
	}()

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
