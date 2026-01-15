// Where: cli/internal/constants/env.go
// What: Environment variable naming constants.
// Why: Centralize environment variable names to avoid typos and inconsistencies.
package constants

const (
	// Project Configuration
	EnvESBProjectName = "ESB_PROJECT_NAME"
	EnvESBEnv         = "ESB_ENV"
	EnvESBImageTag    = "ESB_IMAGE_TAG"
	EnvESBMode        = "ESB_MODE"
	EnvESBHome        = "ESB_HOME"
	EnvESBConfigDir   = "ESB_CONFIG_DIR"
	EnvESBInteractive = "ESB_INTERACTIVE"

	// Network Configuration
	EnvSubnetExternal   = "ESB_SUBNET_EXTERNAL"
	EnvNetworkExternal  = "ESB_NETWORK_EXTERNAL"
	EnvRuntimeNetSubnet = "RUNTIME_NET_SUBNET"
	EnvRuntimeNodeIP    = "RUNTIME_NODE_IP"
	EnvLambdaNetwork    = "LAMBDA_NETWORK"

	// Port Configuration
	EnvPortGatewayHTTPS = "ESB_PORT_GATEWAY_HTTPS"
	EnvPortGatewayHTTP  = "ESB_PORT_GATEWAY_HTTP"
	EnvPortAgentCGRPC   = "ESB_PORT_AGENT_GRPC"
	EnvPortS3           = "ESB_PORT_S3"
	EnvPortS3Mgmt       = "ESB_PORT_S3_MGMT"
	EnvPortDatabase     = "ESB_PORT_DATABASE"
	EnvPortRegistry     = "ESB_PORT_REGISTRY"
	EnvPortVictoriaLogs = "ESB_PORT_VICTORIALOGS"

	// Legacy/External Service Ports (Gateway Internal)
	EnvGatewayPort        = "GATEWAY_PORT"
	EnvGatewayURL         = "GATEWAY_URL"
	EnvGatewayInternalURL = "GATEWAY_INTERNAL_URL"
	EnvVictoriaLogsPort   = "VICTORIALOGS_PORT"
	EnvVictoriaLogsURL    = "VICTORIALOGS_URL"
	EnvAgentGrpcAddress   = "AGENT_GRPC_ADDRESS"

	// Generator Configuration
	EnvGatewayFunctionsYml = "GATEWAY_FUNCTIONS_YML"
	EnvGatewayRoutingYml   = "GATEWAY_ROUTING_YML"

	// Registry Configuration
	EnvContainerRegistry = "CONTAINER_REGISTRY"
)
