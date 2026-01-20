package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/go-cni"
	"github.com/docker/docker/client"
	"github.com/poruru/edge-serverless-box/meta"
	"github.com/poruru/edge-serverless-box/services/agent/internal/api"
	cni_gen "github.com/poruru/edge-serverless-box/services/agent/internal/cni"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
	agentContainerd "github.com/poruru/edge-serverless-box/services/agent/internal/runtime/containerd"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime/docker"
	pb "github.com/poruru/edge-serverless-box/services/agent/pkg/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
	esbEnv := os.Getenv(meta.EnvVarEnv)
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

		wrappedClient := &agentContainerd.ClientWrapper{Client: c}

		cniConfDir := os.Getenv("CNI_CONF_DIR")
		if cniConfDir == "" {
			cniConfDir = "/etc/cni/net.d"
		}

		cniSubnet := strings.TrimSpace(os.Getenv("CNI_SUBNET"))

		// Dynamically generate CNI configuration based on branding constants
		if err := cni_gen.GenerateConfig(cniConfDir, cniSubnet); err != nil {
			log.Printf("WARN: Failed to generate dynamic CNI config: %v", err)
		}

		cniConfFile := os.Getenv("CNI_CONF_FILE")
		if cniConfFile == "" {
			cniConfFile = fmt.Sprintf("%s/10-%s.conflist", cniConfDir, meta.RuntimeCNIName)
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
		namespace := meta.RuntimeNamespace
		rt = agentContainerd.NewRuntime(wrappedClient, cniPlugin, namespace, esbEnv)
		log.Printf("Runtime: containerd (initialized with CNI, namespace=%s)", namespace)

	} else {
		log.Println("Initializing Docker Runtime...")

		// Initialize Docker Client
		dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			log.Fatalf("Failed to create Docker client: %v", err)
		}

		rt = docker.NewRuntime(dockerCli, networkID, esbEnv)
		log.Println("Runtime: docker (initialized)")

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

	grpcOptions, err := grpcServerOptions()
	if err != nil {
		log.Fatalf("Failed to initialize gRPC server options: %v", err)
	}
	if os.Getenv("AGENT_GRPC_TLS_ENABLED") != "1" {
		log.Println("WARNING: gRPC TLS is disabled (AGENT_GRPC_TLS_ENABLED!=1). Use only in trusted networks.")
	}
	grpcServer := grpc.NewServer(grpcOptions...)
	agentServer := api.NewAgentServer(rt)
	pb.RegisterAgentServiceServer(grpcServer, agentServer)

	if isReflectionEnabled() {
		// Enable reflection for debugging (grpcurl etc.)
		reflection.Register(grpcServer)
	}

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

func isReflectionEnabled() bool {
	return os.Getenv("AGENT_GRPC_REFLECTION") == "1"
}

func grpcServerOptions() ([]grpc.ServerOption, error) {
	if os.Getenv("AGENT_GRPC_TLS_ENABLED") != "1" {
		return nil, nil
	}

	certPath := strings.TrimSpace(os.Getenv("AGENT_GRPC_CERT_PATH"))
	if certPath == "" {
		certPath = "/app/config/ssl/server.crt"
	}
	keyPath := strings.TrimSpace(os.Getenv("AGENT_GRPC_KEY_PATH"))
	if keyPath == "" {
		keyPath = "/app/config/ssl/server.key"
	}
	caPath := strings.TrimSpace(os.Getenv("AGENT_GRPC_CA_CERT_PATH"))
	if caPath == "" {
		caPath = meta.RootCACertPath
	}

	certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load server cert: %w", err)
	}

	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}

	caPool, err := x509.SystemCertPool()
	if err != nil {
		caPool = x509.NewCertPool()
	}
	if ok := caPool.AppendCertsFromPEM(caPEM); !ok {
		return nil, fmt.Errorf("append CA cert failed")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{certificate},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}

	return []grpc.ServerOption{grpc.Creds(credentials.NewTLS(tlsConfig))}, nil
}
