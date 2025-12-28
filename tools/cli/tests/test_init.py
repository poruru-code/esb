"""Unit tests for esb init command"""
import sys
import yaml
import pytest
from pathlib import Path
from unittest.mock import patch, MagicMock, PropertyMock
from argparse import Namespace

from tools.cli.commands import init


@pytest.fixture
def mock_template_yaml():
    return """
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31
Parameters:
  Prefix:
    Type: String
    Default: dev
    Description: Environment prefix
  AccessLogGroup:
    Type: String
    Default: /aws/lambda/access-logs
"""


def test_init_aborts_on_cancelled_input(tmp_path):
    """Exit when questionary is canceled (None)."""
    # Run in a directory without template.yaml.
    with patch("tools.cli.commands.init.cli_config") as mock_config, \
         patch("tools.cli.commands.init.questionary") as mock_questionary:
        
        mock_config.TEMPLATE_YAML = None
        # questionary.path().ask() returns None.
        mock_questionary.path.return_value.ask.return_value = None
        
        args = Namespace(template=None)
        with pytest.raises(SystemExit) as exc:
            init.run(args)
        assert exc.value.code == 1


def test_init_uses_cli_config_template(mock_template_yaml, tmp_path):
    """Prefer cli_config.TEMPLATE_YAML."""
    template_file = tmp_path / "template.yaml"
    template_file.write_text(mock_template_yaml)
    
    with patch("tools.cli.commands.init.cli_config") as mock_config, \
         patch("tools.cli.commands.init.questionary.text") as mock_text, \
         patch("tools.cli.commands.init.questionary.path") as mock_path, \
         patch("tools.cli.commands.init.questionary.confirm") as mock_confirm:
        
        # Set cli_config.TEMPLATE_YAML.
        mock_config.TEMPLATE_YAML = template_file
        
        # questionary mocks.
        mock_text.return_value.ask.side_effect = ["prod", "/aws/logs", "latest"]
        mock_path.return_value.ask.return_value = str(tmp_path / ".esb")
        mock_confirm.return_value.ask.return_value = True  # Overwrite.
        
        args = Namespace(template=None)
        init.run(args)
    
    # Ensure generator.yml is created.
    gen_file = tmp_path / "generator.yml"
    assert gen_file.exists()
    
    with open(gen_file) as f:
        config = yaml.safe_load(f)
    
    # Ensure portable relative paths are generated.
    assert config["paths"]["sam_template"] == "template.yaml"
    assert config["paths"]["output_dir"] == ".esb/"


def test_init_generates_portable_paths(mock_template_yaml, tmp_path):
    """Ensure generated generator.yml contains portable relative paths."""
    template_file = tmp_path / "template.yaml"
    template_file.write_text(mock_template_yaml)
    
    with patch("tools.cli.commands.init.cli_config") as mock_config, \
         patch("tools.cli.commands.init.questionary.text") as mock_text, \
         patch("tools.cli.commands.init.questionary.path") as mock_path:
        
        mock_config.TEMPLATE_YAML = template_file
        mock_text.return_value.ask.side_effect = ["dev", "/aws/logs", "v1.0"]
        mock_path.return_value.ask.return_value = str(tmp_path / ".esb")
        
        args = Namespace(template=None)
        init.run(args)
    
    with open(tmp_path / "generator.yml") as f:
        config = yaml.safe_load(f)
    
    assert config["app"]["tag"] == "v1.0"
    assert config["parameters"]["Prefix"] == "dev"


def test_init_skips_empty_parameters(tmp_path):
    """Omit the parameters section when the template has no Parameters."""
    template_content = """
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31
Resources:
  MyFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: test-function
      Runtime: python3.12
      Handler: app.handler
      CodeUri: ./src
"""
    template_file = tmp_path / "template.yaml"
    template_file.write_text(template_content)
    
    with patch("tools.cli.commands.init.cli_config") as mock_config, \
         patch("tools.cli.commands.init.questionary.text") as mock_text, \
         patch("tools.cli.commands.init.questionary.path") as mock_path:
        
        mock_config.TEMPLATE_YAML = template_file
        mock_text.return_value.ask.return_value = "latest"
        mock_path.return_value.ask.return_value = str(tmp_path / ".esb")
        
        args = Namespace(template=None)
        init.run(args)
    
    with open(tmp_path / "generator.yml") as f:
        config = yaml.safe_load(f)
    
    # Ensure the parameters key does not exist.
    assert "parameters" not in config


def test_init_respects_overwrite_cancel(mock_template_yaml, tmp_path):
    """Exit when choosing No on overwrite confirmation."""
    template_file = tmp_path / "template.yaml"
    template_file.write_text(mock_template_yaml)
    
    # Create an existing generator.yml.
    existing_gen = tmp_path / "generator.yml"
    existing_gen.write_text("existing: true")
    
    with patch("tools.cli.commands.init.cli_config") as mock_config, \
         patch("tools.cli.commands.init.questionary.text") as mock_text, \
         patch("tools.cli.commands.init.questionary.path") as mock_path, \
         patch("tools.cli.commands.init.questionary.confirm") as mock_confirm:
        
        mock_config.TEMPLATE_YAML = template_file
        mock_text.return_value.ask.side_effect = ["dev", "/aws/logs", "latest"]
        mock_path.return_value.ask.return_value = str(tmp_path / ".esb")
        mock_confirm.return_value.ask.return_value = False  # Do not overwrite.
        
        args = Namespace(template=None)
        with pytest.raises(SystemExit) as exc:
            init.run(args)
        assert exc.value.code == 0
    
    # Ensure the file was not overwritten.
    with open(existing_gen) as f:
        assert f.read() == "existing: true"
