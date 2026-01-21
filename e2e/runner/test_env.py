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

        assert env["ENV"] == "myenv"
        assert env["MODE"] == "docker"
        assert env["PROJECT_NAME"] == "myproj"
        assert env["IMAGE_TAG"] == "docker"
        assert env["IMAGE_PREFIX"] == "myproj"

        # Check Subnets are present
        assert "SUBNET_EXTERNAL" in env
        assert "RUNTIME_NET_SUBNET" in env
        assert "RUNTIME_NODE_IP" in env
        assert env["NETWORK_EXTERNAL"] == "myproj-myenv-external"
        assert env["LAMBDA_NETWORK"] == "esb_int_myenv"

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
        assert env["AUTH_USER"] == BRAND_SLUG
        assert len(env["AUTH_PASS"]) == 32  # token_hex(16) -> 32 chars
        assert len(env["JWT_SECRET_KEY"]) == 64
        assert len(env["X_API_KEY"]) == 64

        # Check Branding & Repo Path
        assert env["ENV_PREFIX"] == ENV_PREFIX
        assert env["CLI_CMD"] == BRAND_SLUG


def test_calculate_runtime_env_override():
    # Ensure calculate_runtime_env respects existing environment variables
    # We use env_key to ensure we match whatever prefix logic is active
    auth_key = "AUTH_USER"
    gw_port_key = env_key("PORT_GATEWAY_HTTPS")

    with mock.patch.dict(os.environ, {auth_key: "alice", gw_port_key: "8443"}, clear=True):
        env = calculate_runtime_env("myproj", "myenv", "docker")

        assert env[auth_key] == "alice"
        assert env[gw_port_key] == "8443"


def test_calculate_runtime_env_mode_tags():
    # docker mode
    env_docker = calculate_runtime_env("p", "e", "docker")
    assert env_docker["IMAGE_TAG"] == "docker"

    # containerd mode
    env_containerd = calculate_runtime_env("p", "e", "containerd")
    assert env_containerd["IMAGE_TAG"] == "containerd"
    assert env_containerd["CONTAINER_REGISTRY"] == "registry:5010"

    # custom mode (should fallback to env name or latest)
    env_custom = calculate_runtime_env("p", "prod", "firecracker")
    assert env_custom["IMAGE_TAG"] == "firecracker"


def test_calculate_staging_dir_logic():
    path = calculate_staging_dir("myproj", "myenv")
    assert "myproj" in str(path)
    assert "myenv" in str(path)
    assert ".cache/staging" in str(path)


def test_calculate_runtime_env_project_config(tmp_path):
    # Test that project-specific config like CONFIG_DIR and FunctionsYml are set
    project = "myproj"
    env_name = "myenv"

    # Create fake project repo for generator.yml
    repo = tmp_path / "repo"
    repo.mkdir()
    gen_yml = repo / "generator.yml"
    gen_yml.write_text("paths:\n  functions_yml: custom_fns.yml\n")

    # Mock calculate_staging_dir to return a path in tmp_path
    staging_dir = tmp_path / "staging"
    staging_dir.mkdir(parents=True)

    with (
        mock.patch("e2e.runner.env.calculate_staging_dir") as mock_calc,
        mock.patch.dict(os.environ, {env_key("REPO"): str(repo)}, clear=True),
    ):
        mock_calc.return_value = staging_dir

        env = calculate_runtime_env(project, env_name, "docker")

        assert "CONFIG_DIR" in env
        assert env["CONFIG_DIR"] == str(staging_dir)
        assert env.get("GATEWAY_FUNCTIONS_YML") == "custom_fns.yml"
