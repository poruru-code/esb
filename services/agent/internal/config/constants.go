package config

import "time"

const (
	// Core
	DefaultPort        = "50051"
	DefaultMetricsPort = "9091"
	DefaultEnv         = "default"
	DefaultRuntime     = "docker"
	DefaultNetwork     = "bridge"

	// gRPC
	DefaultGRPCReflection  = "0"
	DefaultGRPCTLSDisabled = "0"

	// Logging
	DefaultLogLevel  = "info"
	DefaultLogFormat = "text"

	// Limits
	DefaultMaxResponseSize = 10 * 1024 * 1024 // 10MB

	// Registry
	DefaultContainerRegistry = "registry:5010"

	// gRPC Security
	DefaultCertPath = "/app/config/ssl/server.crt"
	DefaultKeyPath  = "/app/config/ssl/server.key"

	// CNI / Networking
	DefaultCNIConfDir   = "/etc/cni/net.d"
	DefaultCNIBinDir    = "/opt/cni/bin"
	DefaultCNIDNSServer = "10.88.0.1"
	DefaultCNISubnet    = "10.88.0.0/16"

	// Containerd
	DefaultContainerdSocket     = "/run/containerd/containerd.sock"
	DefaultSnapshotterOverlay   = "overlayfs"
	DefaultSnapshotterDevmapper = "devmapper"

	// Timeouts
	DefaultMetricsReadHeaderTimeout = 5 * time.Second
)
