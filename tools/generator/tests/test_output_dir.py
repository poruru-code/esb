"""Unit tests for generator output_dir path resolution"""
import pytest
import yaml
import shutil
from pathlib import Path
from unittest.mock import patch, MagicMock

from tools.generator import main as generator_main


@pytest.fixture
def sample_template():
    """Sample SAM template."""
    return """
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31
Resources:
  TestFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: test-function
      Runtime: python3.12
      Handler: app.handler
      CodeUri: ./src/
"""


@pytest.fixture
def sample_function_code():
    """Sample function code."""
    return """
def handler(event, context):
    return {"statusCode": 200, "body": "Hello"}
"""


def test_output_dir_relative_to_template(tmp_path, sample_template, sample_function_code):
    """Ensure output_dir resolves correctly as a relative path from the template."""
    # Setup: template directory structure.
    template_dir = tmp_path / "project"
    template_dir.mkdir()
    
    # template.yaml
    template_file = template_dir / "template.yaml"
    template_file.write_text(sample_template)
    
    # src/app.py
    src_dir = template_dir / "src"
    src_dir.mkdir()
    (src_dir / "app.py").write_text(sample_function_code)
    
    # generator.yml (output_dir specified as a relative path).
    config = {
        "app": {"name": "", "tag": "latest"},
        "paths": {
            "sam_template": "template.yaml",
            "output_dir": "build/"  # Relative path from template.
        }
    }
    config_file = template_dir / "generator.yml"
    with open(config_file, "w") as f:
        yaml.dump(config, f)
    
    # Execute
    functions = generator_main.generate_files(
        config=config,
        project_root=template_dir,
        dry_run=False,
        verbose=False
    )
    
    # Assert: files are generated inside output_dir.
    output_dir = template_dir / "build"
    assert output_dir.exists(), "output_dir was not created"
    
    # functions.yml should be generated in output_dir/config/.
    config_dir = output_dir / "config"
    assert config_dir.exists(), "config/ directory was not created"
    assert (config_dir / "functions.yml").exists(), "functions.yml was not generated"
    assert (config_dir / "routing.yml").exists(), "routing.yml was not generated"
    
    # Dockerfile should be generated in output_dir/functions/<name>/.
    func_staging = output_dir / "functions" / "test-function"
    assert func_staging.exists(), "function staging directory was not created"
    assert (func_staging / "Dockerfile").exists(), "Dockerfile was not generated"


def test_output_dir_absolute_path(tmp_path, sample_template, sample_function_code):
    """Ensure output_dir works correctly when it is an absolute path."""
    # Setup
    template_dir = tmp_path / "project"
    template_dir.mkdir()
    
    template_file = template_dir / "template.yaml"
    template_file.write_text(sample_template)
    
    src_dir = template_dir / "src"
    src_dir.mkdir()
    (src_dir / "app.py").write_text(sample_function_code)
    
    # Specify a different directory as output_dir (absolute path).
    output_dir = tmp_path / "separate_output"
    
    config = {
        "app": {"name": "", "tag": "latest"},
        "paths": {
            "sam_template": str(template_file),
            "output_dir": str(output_dir) + "/"
        }
    }
    
    # Execute
    functions = generator_main.generate_files(
        config=config,
        project_root=template_dir,
        dry_run=False,
        verbose=False
    )
    
    # Assert: output is created in a separate directory.
    assert output_dir.exists(), "output_dir (absolute path) was not created"
    assert (output_dir / "config" / "functions.yml").exists()
    assert (output_dir / "functions" / "test-function" / "Dockerfile").exists()


def test_output_dir_deep_nested(tmp_path, sample_template, sample_function_code):
    """Ensure deeply nested output_dir works correctly."""
    # Setup
    template_dir = tmp_path / "project"
    template_dir.mkdir()
    
    template_file = template_dir / "template.yaml"
    template_file.write_text(sample_template)
    
    src_dir = template_dir / "src"
    src_dir.mkdir()
    (src_dir / "app.py").write_text(sample_function_code)
    
    config = {
        "app": {"name": "", "tag": "latest"},
        "paths": {
            "sam_template": "template.yaml",
            "output_dir": "deep/nested/output/"  # Deep nesting.
        }
    }
    
    # Execute
    functions = generator_main.generate_files(
        config=config,
        project_root=template_dir,
        dry_run=False,
        verbose=False
    )
    
    # Assert
    output_dir = template_dir / "deep" / "nested" / "output"
    assert output_dir.exists(), "deeply nested output_dir was not created"
    assert (output_dir / "config" / "functions.yml").exists()


def test_output_dir_dot_esb_relative(tmp_path, sample_template, sample_function_code):
    """Ensure output_dir works when using .esb/ (dot-prefixed)."""
    # Setup
    template_dir = tmp_path / "project"
    template_dir.mkdir()
    
    template_file = template_dir / "template.yaml"
    template_file.write_text(sample_template)
    
    src_dir = template_dir / "src"
    src_dir.mkdir()
    (src_dir / "app.py").write_text(sample_function_code)
    
    config = {
        "app": {"name": "", "tag": "latest"},
        "paths": {
            "sam_template": "template.yaml",
            "output_dir": ".esb/"  # Dot-prefixed (hidden directory).
        }
    }
    
    # Execute
    functions = generator_main.generate_files(
        config=config,
        project_root=template_dir,
        dry_run=False,
        verbose=False
    )
    
    # Assert: created under the template directory.
    output_dir = template_dir / ".esb"
    assert output_dir.exists(), ".esb/ was not resolved relative to the template"
    assert (output_dir / "config" / "functions.yml").exists()


def test_functions_yml_routing_yml_auto_derived(tmp_path, sample_template, sample_function_code):
    """Ensure functions_yml and routing_yml are auto-generated under output_dir/config/."""
    # Setup
    template_dir = tmp_path / "project"
    template_dir.mkdir()
    
    template_file = template_dir / "template.yaml"
    template_file.write_text(sample_template)
    
    src_dir = template_dir / "src"
    src_dir.mkdir()
    (src_dir / "app.py").write_text(sample_function_code)
    
    # Do not explicitly specify functions_yml or routing_yml.
    config = {
        "app": {"name": "", "tag": "latest"},
        "paths": {
            "sam_template": "template.yaml",
            "output_dir": "custom_output/"
        }
    }
    
    # Execute
    functions = generator_main.generate_files(
        config=config,
        project_root=template_dir,
        dry_run=False,
        verbose=False
    )
    
    # Assert: auto-generated under output_dir/config/.
    functions_yml = template_dir / "custom_output" / "config" / "functions.yml"
    routing_yml = template_dir / "custom_output" / "config" / "routing.yml"
    
    assert functions_yml.exists(), "functions.yml was not auto-generated"
    assert routing_yml.exists(), "routing.yml was not auto-generated"
    
    # Verify contents.
    with open(functions_yml) as f:
        content = yaml.safe_load(f)
    assert "functions" in content, "functions.yml missing functions key"
    assert "test-function" in content["functions"], "test-function not included in functions.yml"
