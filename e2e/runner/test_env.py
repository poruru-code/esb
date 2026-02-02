import os
from unittest import mock

from e2e.runner import constants
from e2e.runner.env import (
    calculate_runtime_env,
    calculate_staging_dir,
    env_external_subnet_index,
    env_runtime_subnet_index,
    hash_mod,
)
from e2e.runner.utils import (
    BRAND_SLUG,
    ENV_PREFIX,
    env_key,
)


def test_hash_mod_consistency():
    # Verify deterministic behavior matching the logic ported from Go
    assert hash_mod("foo", 100) == hash_mod("foo", 100)
    assert hash_mod("foo", 100) != hash_mod("bar", 100)
    assert 0 <= hash_mod("test", 100) < 100


def test_subnet_indices_default():
    assert env_external_subnet_index("default") == 50
    assert env_runtime_subnet_index("default") == 20


def test_subnet_indices_custom():
    # Verify range consistency
    idx_ext = env_external_subnet_index("custom-env")
    idx_run = env_runtime_subnet_index("custom-env")
    assert 60 <= idx_ext < 160
    assert 100 <= idx_run < 200


def test_calculate_runtime_env_defaults():
    # Test strict defaults for a new environment
    with mock.patch.dict(os.environ, {}, clear=True):
        env = calculate_runtime_env("myproj", "myenv", "docker")

        assert env[constants.ENV_ENV] == "myenv"
        assert env[constants.ENV_MODE] == "docker"
        assert env[constants.ENV_PROJECT_NAME] == "myproj"
        tag_key = env_key(constants.ENV_TAG)
        assert env[tag_key] == "latest"

        # Check Subnets are present
        assert constants.ENV_SUBNET_EXTERNAL in env
        assert constants.ENV_RUNTIME_NET_SUBNET in env
        assert constants.ENV_RUNTIME_NODE_IP in env
        assert env[constants.ENV_NETWORK_EXTERNAL] == "myproj-myenv-external"
        assert env[constants.ENV_LAMBDA_NETWORK] == "esb_int_myenv"

        # Check Ports (should be initialized to "0" for dynamic discovery)
        for p_suffix in (
            constants.PORT_GATEWAY_HTTPS,
            constants.PORT_GATEWAY_HTTP,
            constants.PORT_AGENT_GRPC,
            constants.PORT_S3,
            constants.PORT_S3_MGMT,
            constants.PORT_DATABASE,
            constants.PORT_REGISTRY,
            constants.PORT_VICTORIALOGS,
        ):
            key = env_key(p_suffix)
            assert env[key] == "0"

        # Check Credentials generation matches branding
        assert env[constants.ENV_AUTH_USER] == BRAND_SLUG
        assert len(env[constants.ENV_AUTH_PASS]) == 32  # token_hex(16) -> 32 chars
        assert len(env[constants.ENV_JWT_SECRET_KEY]) == 64
        assert len(env[constants.ENV_X_API_KEY]) == 64

        # Check Branding & Repo Path
        assert env["ENV_PREFIX"] == ENV_PREFIX
        assert env[constants.ENV_CLI_CMD] == BRAND_SLUG


def test_calculate_runtime_env_override():
    # Ensure calculate_runtime_env respects existing environment variables
    # We use env_key to ensure we match whatever prefix logic is active
    auth_key = constants.ENV_AUTH_USER
    gw_port_key = env_key(constants.PORT_GATEWAY_HTTPS)

    with mock.patch.dict(os.environ, {auth_key: "alice", gw_port_key: "8443"}, clear=True):
        env = calculate_runtime_env("myproj", "myenv", "docker")

        assert env[auth_key] == "alice"
        assert env[gw_port_key] == "8443"


def test_calculate_runtime_env_mode_registry_defaults():
    # docker mode: container registry defaults to host registry
    registry_key = env_key(constants.ENV_REGISTRY)
    registry_port_key = env_key(constants.PORT_REGISTRY)
    with mock.patch.dict(os.environ, {registry_port_key: "5010"}, clear=True):
        env_docker = calculate_runtime_env("p", "e", "docker")
        assert env_docker[registry_key] == "127.0.0.1:5010/"
        assert env_docker[constants.ENV_CONTAINER_REGISTRY] == "127.0.0.1:5010"

    # containerd mode: registry is required and normalized
    env_containerd = calculate_runtime_env("p", "e", "containerd")
    assert env_containerd[registry_key] == "registry:5010/"
    assert env_containerd[constants.ENV_CONTAINER_REGISTRY] == "registry:5010"


def test_calculate_staging_dir_logic():
    path = calculate_staging_dir("myproj", "myenv")
    assert "myproj" in str(path)
    assert "myenv" in str(path)
    assert ".cache/staging" in str(path)


def test_calculate_runtime_env_project_config_dir(tmp_path):
    # Test that project-specific config like CONFIG_DIR is set
    project = "myproj"
    env_name = "myenv"

    # Mock calculate_staging_dir to return a path in tmp_path
    staging_dir = tmp_path / "staging"
    staging_dir.mkdir(parents=True)

    with (
        mock.patch("e2e.runner.env.calculate_staging_dir") as mock_calc,
        mock.patch.dict(os.environ, {}, clear=True),
    ):
        mock_calc.return_value = staging_dir

        env = calculate_runtime_env(project, env_name, "docker")

        assert constants.ENV_CONFIG_DIR in env
        assert env[constants.ENV_CONFIG_DIR] == str(staging_dir)
