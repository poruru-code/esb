package constants

const (
	// Compose variables (no prefix).
	EnvProjectName       = "PROJECT_NAME"
	EnvPortS3            = "PORT_S3"
	EnvPortS3Mgmt        = "PORT_S3_MGMT"
	EnvPortDatabase      = "PORT_DATABASE"
	EnvPortGatewayHTTPS  = "PORT_GATEWAY_HTTPS"
	EnvPortGatewayHTTP   = "PORT_GATEWAY_HTTP"
	EnvPortAgentGRPC     = "PORT_AGENT_GRPC"
	EnvPortRegistry      = "PORT_REGISTRY"
	EnvPortVictoriaLogs  = "PORT_VICTORIALOGS"
	EnvPortAgentMetrics  = "PORT_AGENT_METRICS"
	EnvNetworkExternal   = "NETWORK_EXTERNAL"
	EnvSubnetExternal    = "SUBNET_EXTERNAL"
	EnvRuntimeNetSubnet  = "RUNTIME_NET_SUBNET"
	EnvRuntimeNodeIP     = "RUNTIME_NODE_IP"
	EnvLambdaNetwork     = "LAMBDA_NETWORK"
	EnvContainerRegistry = "CONTAINER_REGISTRY"
	EnvConfigDir         = "CONFIG_DIR"
	EnvRootCAMountID     = "ROOT_CA_MOUNT_ID"
	EnvBuildkitdConfig   = "BUILDKITD_CONFIG"

	// Host variable suffixes (used with envutil.HostEnvKey).
	HostSuffixEnv              = "ENV"
	HostSuffixMode             = "MODE"
	HostSuffixProject          = "PROJECT"
	HostSuffixHome             = "HOME"
	HostSuffixRepo             = "REPO"
	HostSuffixTemplate         = "TEMPLATE"
	HostSuffixConfigDir        = "CONFIG_DIR"
	HostSuffixInteractive      = "INTERACTIVE"
	HostSuffixNoProxyExtra     = "NO_PROXY_EXTRA"
	HostSuffixCACertPath       = "CA_CERT_PATH"
	HostSuffixCertDir          = "CERT_DIR"
	HostSuffixTag              = "TAG"
	HostSuffixRegistry         = "REGISTRY"
	HostSuffixProvisionerTrace = "PROVISIONER_TRACE"

	// Default registry (internal service name).
	DefaultContainerRegistry = "registry:5010"
	// Default registry (host access for docker mode).
	DefaultContainerRegistryHost = "127.0.0.1:5010"

	// Old Constants (kept temporarily for migration if needed, but renamed/cleaned up)
	// These are actually the same keys but the constant name is renamed.
	// Most will be replaced by dynamic keys using suffixes above.

	// Port Configuration (legacy mappings).
	EnvGatewayPort        = "GATEWAY_PORT"
	EnvGatewayURL         = "GATEWAY_URL"
	EnvGatewayInternalURL = "GATEWAY_INTERNAL_URL"
	EnvVictoriaLogsPort   = "VICTORIALOGS_PORT"
	EnvVictoriaLogsURL    = "VICTORIALOGS_URL"
	EnvAgentMetricsPort   = "AGENT_METRICS_PORT"
	EnvAgentMetricsURL    = "AGENT_METRICS_URL"
	EnvAgentGrpcAddress   = "AGENT_GRPC_ADDRESS"

	// Generator Configuration.
	EnvGatewayFunctionsYml = "GATEWAY_FUNCTIONS_YML"
	EnvGatewayRoutingYml   = "GATEWAY_ROUTING_YML"

	// Build args.
	BuildArgCAFingerprint = "ROOT_CA_FINGERPRINT"
)
