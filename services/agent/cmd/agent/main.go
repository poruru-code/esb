package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/go-cni"
	"github.com/docker/docker/client"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/poruru/edge-serverless-box/services/agent/internal/api"
	cni_gen "github.com/poruru/edge-serverless-box/services/agent/internal/cni"
	"github.com/poruru/edge-serverless-box/services/agent/internal/config"
	"github.com/poruru/edge-serverless-box/services/agent/internal/identity"
	"github.com/poruru/edge-serverless-box/services/agent/internal/interceptor"
	"github.com/poruru/edge-serverless-box/services/agent/internal/logger"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
	agentContainerd "github.com/poruru/edge-serverless-box/services/agent/internal/runtime/containerd"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime/docker"
	pb "github.com/poruru/edge-serverless-box/services/agent/pkg/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	// Prometheus

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	logger.Init()
	slog.Info("Starting ESB Agent...")

	// Configuration
	port := os.Getenv("PORT")
	if port == "" {
		port = config.DefaultPort
	}

	// Network Configuration
	networkID := os.Getenv("CONTAINERS_NETWORK")
	if networkID == "" {
		networkID = config.DefaultNetwork
		slog.Warn("CONTAINERS_NETWORK not specified, defaulting", "network", networkID)
	}
	slog.Info("Target Network", "network", networkID)

	// Phase 7: Environment Isolation
	esbEnv := os.Getenv(identity.EnvName)
	if esbEnv == "" {
		esbEnv = config.DefaultEnv
	}
	slog.Info("ESB Environment", "env", esbEnv)

	resolvedIdentity := identity.ResolveStackIdentityFrom(
		strings.TrimSpace(os.Getenv(identity.EnvBrandSlug)),
		strings.TrimSpace(os.Getenv(identity.EnvProjectName)),
		esbEnv,
		networkID,
	)
	runtime.ApplyIdentity(resolvedIdentity)
	slog.Info(
		"Resolved stack identity",
		"brand", resolvedIdentity.BrandSlug,
		"source", resolvedIdentity.Source,
		"project", strings.TrimSpace(os.Getenv(identity.EnvProjectName)),
		"network", networkID,
	)

	// Initialize Runtime
	var rt runtime.ContainerRuntime
	var dockerCli *client.Client
	var err error
	namespace := resolvedIdentity.RuntimeNamespace()

	runtimeType := os.Getenv("AGENT_RUNTIME")
	if runtimeType == "" {
		runtimeType = config.DefaultRuntime
	}

	if runtimeType == "containerd" {
		slog.Info("Initializing containerd Runtime...")

		// 1. Initialize containerd client
		// Assumes /run/containerd/containerd.sock is mounted
		containerdSocket := os.Getenv("CONTAINERD_SOCKET")
		if containerdSocket == "" {
			containerdSocket = config.DefaultContainerdSocket
		}
		c, err := containerd.New(containerdSocket)
		if err != nil {
			slog.Error("Failed to create containerd client", "socket", containerdSocket, "error", err)
			os.Exit(1)
		}

		wrappedClient := &agentContainerd.ClientWrapper{Client: c}

		cniConfDir := os.Getenv("CNI_CONF_DIR")
		if cniConfDir == "" {
			cniConfDir = config.DefaultCNIConfDir
		}

		cniSubnet := strings.TrimSpace(os.Getenv("CNI_SUBNET"))

		// Dynamically generate CNI configuration based on resolved stack identity.
		if err := cni_gen.GenerateConfig(
			cniConfDir,
			cniSubnet,
			resolvedIdentity.RuntimeCNIName(),
			resolvedIdentity.RuntimeCNIBridge(),
		); err != nil {
			slog.Warn("Failed to generate dynamic CNI config", "error", err)
		}

		cniConfFile := os.Getenv("CNI_CONF_FILE")
		if cniConfFile == "" {
			cniConfFile = fmt.Sprintf("%s/10-%s.conflist", cniConfDir, resolvedIdentity.RuntimeCNIName())
		}

		cniBinDir := os.Getenv("CNI_BIN_DIR")
		if cniBinDir == "" {
			cniBinDir = config.DefaultCNIBinDir
		}

		cniPlugin, err := cni.New(
			cni.WithPluginConfDir(cniConfDir),
			cni.WithPluginDir([]string{cniBinDir}),
			cni.WithInterfacePrefix("eth"),
			cni.WithMinNetworkCount(1),
		)
		if err != nil {
			slog.Error("Failed to initialize CNI", "error", err)
			os.Exit(1)
		}

		if err := cniPlugin.Load(cni.WithConfListFile(cniConfFile)); err != nil {
			slog.Error("Failed to load CNI config", "file", cniConfFile, "error", err)
			os.Exit(1)
		}

		// 2. Create Runtime with CNI networking
		rt = agentContainerd.NewRuntime(
			wrappedClient,
			cniPlugin,
			namespace,
			esbEnv,
			resolvedIdentity.RuntimeContainerPrefix(),
		)
		slog.Info("Runtime initialized", "runtime", "containerd", "namespace", namespace)

	} else {
		slog.Info("Initializing Docker Runtime...")

		// Initialize Docker Client
		dockerCli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			slog.Error("Failed to create Docker client", "error", err)
			os.Exit(1)
		}

		rt = docker.NewRuntime(dockerCli, networkID, esbEnv, resolvedIdentity.RuntimeContainerPrefix())
		slog.Info("Runtime initialized", "runtime", "docker")

		ctx := context.Background()
		info, err := dockerCli.Info(ctx)
		if err != nil {
			slog.Error("Failed to connect to Docker daemon", "error", err)
			os.Exit(1)
		}
		slog.Info("Connected to Docker", "version", info.ServerVersion)
	}

	defer func() {
		if rt != nil {
			rt.Close()
		}
	}()

	// Initialize gRPC Server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		slog.Error("Failed to listen", "port", port, "error", err)
		os.Exit(1)
	}

	grpcOptions, err := grpcServerOptions()
	if err != nil {
		slog.Error("Failed to initialize gRPC server options", "error", err)
		os.Exit(1)
	}
	grpcTLSDisabled := os.Getenv("AGENT_GRPC_TLS_DISABLED")
	if grpcTLSDisabled == "" {
		grpcTLSDisabled = config.DefaultGRPCTLSDisabled
	}
	if grpcTLSDisabled == "1" {
		slog.Warn("gRPC TLS is explicitly disabled (AGENT_GRPC_TLS_DISABLED=1). Use only in trusted networks.")
	} else {
		slog.Info("gRPC TLS is enabled by default.")
	}

	// Setup logging interceptor
	loggingOpts := []logging.Option{
		logging.WithLogOnEvents(logging.FinishCall),
	}
	grpcOptions = append(grpcOptions, grpc.ChainUnaryInterceptor(
		logging.UnaryServerInterceptor(interceptor.Logger(slog.Default()), loggingOpts...),
		grpc_prometheus.UnaryServerInterceptor,
	))

	grpcServer := grpc.NewServer(grpcOptions...)
	grpc_prometheus.Register(grpcServer)
	grpc_prometheus.EnableHandlingTimeHistogram()

	agentServer := api.NewAgentServer(rt)
	pb.RegisterAgentServiceServer(grpcServer, agentServer)

	// Register gRPC health service
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("agent.v1.AgentService", healthpb.HealthCheckResponse_SERVING)

	if isReflectionEnabled() {
		// Enable reflection for debugging (grpcurl etc.)
		reflection.Register(grpcServer)
	}

	// Start Prometheus metrics server
	metricsPort := os.Getenv("AGENT_METRICS_PORT")
	if metricsPort == "" {
		metricsPort = config.DefaultMetricsPort
	}
	metricsServer := &http.Server{
		Addr:              ":" + metricsPort,
		Handler:           promhttp.Handler(),
		ReadHeaderTimeout: config.DefaultMetricsReadHeaderTimeout,
	}
	go func() {
		slog.Info("Starting Prometheus metrics server", "port", metricsPort)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Failed to start metrics server", "error", err)
		}
	}()

	// Signal handling for graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		slog.Info("Received shutdown signal, cleaning up...")

		// Perform GC before shutdown
		if err := rt.GC(context.Background()); err != nil {
			slog.Warn("GC during shutdown failed", "error", err)
		}

		// Graceful shutdown of metrics server
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			slog.Warn("Metrics server shutdown failed", "error", err)
		}

		slog.Info("Shutting down gRPC server...")
		grpcServer.GracefulStop()
	}()

	slog.Info("gRPC server listening", "port", port)
	if err := grpcServer.Serve(lis); err != nil {
		slog.Error("Failed to serve", "error", err)
		os.Exit(1)
	}
}

func isReflectionEnabled() bool {
	val := os.Getenv("AGENT_GRPC_REFLECTION")
	if val == "" {
		val = config.DefaultGRPCReflection
	}
	return val == "1"
}

func grpcServerOptions() ([]grpc.ServerOption, error) {
	grpcTLSDisabled := os.Getenv("AGENT_GRPC_TLS_DISABLED")
	if grpcTLSDisabled == "" {
		grpcTLSDisabled = config.DefaultGRPCTLSDisabled
	}
	if grpcTLSDisabled == "1" {
		return nil, nil
	}

	certPath := strings.TrimSpace(os.Getenv("AGENT_GRPC_CERT_PATH"))
	if certPath == "" {
		certPath = config.DefaultCertPath
	}
	keyPath := strings.TrimSpace(os.Getenv("AGENT_GRPC_KEY_PATH"))
	if keyPath == "" {
		keyPath = config.DefaultKeyPath
	}
	caPath := strings.TrimSpace(os.Getenv("AGENT_GRPC_CA_CERT_PATH"))
	if caPath == "" {
		caPath = config.DefaultCACertPath
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
