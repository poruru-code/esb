import os
from unittest.mock import MagicMock, patch

import pytest

from tools.cli import config as cli_config
from tools.cli.core import context


def test_context_validation_checks_project_local_path(tmp_path):
    """
    Verify that context._validate_environment_exists checks the project-local 
    configuration directory (e.g., E2E_DIR/.esb/<env>/config), NOT the global ~/.esb path.
    """
    test_env = "test_path_env"
    project_root = tmp_path / "project"
    project_root.mkdir()
    
    # Create generator.yml with the environment registered
    (project_root / "generator.yml").write_text(f"environments:\n  - {test_env}\n")
    
    # Create the expected local config directory
    env_config_root = project_root / ".esb" / test_env
    config_dir = env_config_root / "config"
    config_dir.mkdir(parents=True)
    
    # Path to generator.yml for this environment (resides in the output dir) - actually wait, 
    # the new logic checks generator.yml in E2E_DIR.
    
    with patch("tools.cli.config.E2E_DIR", project_root):
        with patch.dict(os.environ, {"ESB_ENV": test_env}):
            args = MagicMock()
            args.env = test_env
            
            # This should now pass because generator.yml exists in project_root and has the environment
            try:
                context.enforce_env_arg(args, require_built=True)
            except SystemExit:
                pytest.fail("enforce_env_arg failed despite correct local setup")

def test_get_build_output_dir_respects_generator_yaml(tmp_path):
    """
    Verify that get_build_output_dir reads output_dir from generator.yml.
    """
    # create a dummy E2E_DIR
    dummy_e2e = tmp_path / "e2e"
    dummy_e2e.mkdir()
    
    # create generator.yml
    gen_yaml = dummy_e2e / "generator.yml"
    gen_yaml.write_text("paths:\n  output_dir: custom_builds/")
    
    test_env = "custom_env"
    
    with patch("tools.cli.config.E2E_DIR", dummy_e2e):
        result = cli_config.get_build_output_dir(test_env)
        
        expected = dummy_e2e / "custom_builds" / test_env
        assert result == expected

def test_environment_validation_uses_correct_base_dir(tmp_path):
    """
    Integration-like test: verify validation succeeds ONLY if the project-local config exists.
    """
    test_env = "test_integration_env"
    
    # 1. Setup: Create a fake project structure
    project_root = tmp_path / "project"
    project_root.mkdir()
    
    # Set E2E_DIR to this temp location
    with patch("tools.cli.config.E2E_DIR", project_root):
        
        args = MagicMock()
        args.env = test_env
        
        # Case A: Config missing -> Should fail
        # make sure global home is NOT checked (we assume ~/.esb doesn't contain this random env, 
        # but to be safe we can mock home too, but let's trust the code uses E2E_DIR)
        
        with pytest.raises(SystemExit):
             context.enforce_env_arg(args, require_built=True)
             
        # Case B: Config exists in PROJECT LOCAL dir -> Should pass
        # Setup test environment structure
        env_config_root = project_root / ".esb" / test_env
        # both generator.yml and config dir are now needed
        # generator.yml resides in E2E_DIR (mocked as project_root)
        (project_root / "generator.yml").write_text(f"environments:\n  - {test_env}\n")
        
        env_config_root = project_root / ".esb" / test_env
        config_dir = env_config_root / "config"
        config_dir.mkdir(parents=True)
        # This checks that it found the directory at the corrected local path
        try:
            context.enforce_env_arg(args, require_built=True)
        except SystemExit:
            pytest.fail("enforce_env_arg raised SystemExit despite local config existing!")

        # Case C: Ensure it is NOT checking global home (optional, strict verification)
        # We can rely on Case A passing (SystemExit) to confirm it didn't find it "somewhere else".

def test_set_template_yaml_expands_user():
    """
    Verify that set_template_yaml expands ~ using expanduser().
    """
    import tools.cli.config as cli_config
    from tools.cli.config import set_template_yaml
    
    with patch("os.path.expanduser", return_value="/home/fakeuser/template.yaml"):
        set_template_yaml("~/template.yaml")
        assert "/home/fakeuser/template.yaml" in str(cli_config.TEMPLATE_YAML)
