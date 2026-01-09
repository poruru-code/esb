import os
from unittest.mock import MagicMock, patch

import pytest

from tools.python_cli.commands import build, up


class TestBuildUpIntegration:
    @pytest.fixture
    def mock_args(self):
        args = MagicMock()
        args.detach = True
        args.build = False
        args.wait = False
        args.env = "test-env"
        # up expects 'file' if not using default
        args.file = []
        return args

    @patch("tools.python_cli.commands.up.build.run")
    @patch("tools.python_cli.commands.up.subprocess.check_call")
    @patch("tools.python_cli.commands.up.provisioner.main")
    @patch("tools.python_cli.commands.up.ensure_certs")
    @patch("tools.python_cli.commands.up.proxy.prepare_env")
    @patch("tools.python_cli.commands.up.context.enforce_env_arg")
    def test_up_calls_build_when_flag_present(
        self,
        mock_enforce,
        mock_proxy_env,
        mock_cert,
        mock_prov,
        mock_sub,
        mock_build_run,
        mock_args,
    ):
        """Verify that 'esb up --build' calls 'build.run'"""
        mock_args.build = True

        # We need to mock return value of discover_ports which is imported inline
        with (
            patch("tools.python_cli.core.port_discovery.discover_ports", return_value={}),
            patch("tools.python_cli.core.port_discovery.save_ports", return_value="ports.json"),
            patch("tools.python_cli.core.port_discovery.apply_ports_to_env"),
            patch("tools.python_cli.core.port_discovery.log_ports"),
        ):
            up.run(mock_args)

        # Expectation: build.run is called with the SAME args
        mock_build_run.assert_called_once_with(mock_args)

    @patch("tools.python_cli.commands.build.generator.load_config")
    @patch("tools.python_cli.commands.build.generator.generate_files")
    @patch("tools.python_cli.commands.build.ensure_registry_running")
    @patch("tools.python_cli.commands.build.shutil")
    @patch("tools.python_cli.commands.build.cli_config.get_build_output_dir")
    @patch("tools.python_cli.commands.build.cli_config.get_env_name", return_value="demo")
    def test_build_always_stages_config(
        self,
        mock_get_env,
        mock_get_out,
        mock_shutil,
        mock_ensure_reg,
        mock_gen_files,
        mock_gen_load,
        mock_args,
        tmp_path,
    ):
        """Verify that 'esb build' always triggers staging logic"""
        # Set up output directory and mock it
        mock_out_dir = tmp_path / "out"
        from tools.python_cli import config as cli_lookup

        # Create a mock template file
        mock_template = tmp_path / "template.yaml"
        mock_template.write_text("# mock template")

        # generator.yml resides in E2E_DIR (mocked as tmp_path)
        gen_yml = tmp_path / "generator.yml"
        gen_yml.write_text("environments:\n  - demo\n")

        # We need the source config files to exist for staging to work
        env_out_dir = mock_out_dir / "demo"
        config_dir = env_out_dir / "config"
        config_dir.mkdir(parents=True)
        (config_dir / "functions.yml").write_text("dummy content")
        (config_dir / "routing.yml").write_text("dummy content")

        mock_get_out.side_effect = lambda env: mock_out_dir / env
        mock_gen_load.return_value = {"paths": {}}
        mock_gen_files.return_value = None  # Ensure generator.generate_files is fully mocked

        # Mock TEMPLATE_YAML and E2E_DIR to use tmp_path
        with (
            patch.object(cli_lookup, "TEMPLATE_YAML", mock_template),
            patch.object(cli_lookup, "E2E_DIR", tmp_path),
            patch("tools.python_cli.commands.build.cli_config.TEMPLATE_YAML", mock_template),
            patch("tools.python_cli.commands.build.cli_config.E2E_DIR", tmp_path),
        ):
            build.run(mock_args)

        # Verify ESB_CONFIG_DIR environment variable (now includes env name)
        assert os.environ["ESB_CONFIG_DIR"] == "services/gateway/.esb-staging/demo/config"

        # Verify shutil.copy2 was called
        assert mock_shutil.copy2.called
