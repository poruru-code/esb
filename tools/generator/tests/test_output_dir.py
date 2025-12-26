"""Unit tests for generator output_dir path resolution"""
import pytest
import yaml
import shutil
from pathlib import Path
from unittest.mock import patch, MagicMock

from tools.generator import main as generator_main


@pytest.fixture
def sample_template():
    """サンプルSAMテンプレート"""
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
    """サンプル関数コード"""
    return """
def handler(event, context):
    return {"statusCode": 200, "body": "Hello"}
"""


def test_output_dir_relative_to_template(tmp_path, sample_template, sample_function_code):
    """output_dir がテンプレートからの相対パスで正しく解決されること"""
    # Setup: テンプレートディレクトリ構造
    template_dir = tmp_path / "project"
    template_dir.mkdir()
    
    # template.yaml
    template_file = template_dir / "template.yaml"
    template_file.write_text(sample_template)
    
    # src/app.py
    src_dir = template_dir / "src"
    src_dir.mkdir()
    (src_dir / "app.py").write_text(sample_function_code)
    
    # generator.yml (output_dir を相対パスで指定)
    config = {
        "app": {"name": "", "tag": "latest"},
        "paths": {
            "sam_template": "template.yaml",
            "output_dir": "build/"  # テンプレートからの相対パス
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
    
    # Assert: output_dir 内にファイルが生成されていること
    output_dir = template_dir / "build"
    assert output_dir.exists(), "output_dir が作成されていない"
    
    # functions.yml が output_dir/config/ に生成されること
    config_dir = output_dir / "config"
    assert config_dir.exists(), "config/ ディレクトリが作成されていない"
    assert (config_dir / "functions.yml").exists(), "functions.yml が生成されていない"
    assert (config_dir / "routing.yml").exists(), "routing.yml が生成されていない"
    
    # Dockerfile が output_dir/functions/<name>/ に生成されること
    func_staging = output_dir / "functions" / "test-function"
    assert func_staging.exists(), "function staging ディレクトリが作成されていない"
    assert (func_staging / "Dockerfile").exists(), "Dockerfile が生成されていない"


def test_output_dir_absolute_path(tmp_path, sample_template, sample_function_code):
    """output_dir が絶対パスでも正しく動作すること"""
    # Setup
    template_dir = tmp_path / "project"
    template_dir.mkdir()
    
    template_file = template_dir / "template.yaml"
    template_file.write_text(sample_template)
    
    src_dir = template_dir / "src"
    src_dir.mkdir()
    (src_dir / "app.py").write_text(sample_function_code)
    
    # 別のディレクトリを output_dir として指定 (絶対パス)
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
    
    # Assert: 別ディレクトリに出力されていること
    assert output_dir.exists(), "output_dir (絶対パス) が作成されていない"
    assert (output_dir / "config" / "functions.yml").exists()
    assert (output_dir / "functions" / "test-function" / "Dockerfile").exists()


def test_output_dir_deep_nested(tmp_path, sample_template, sample_function_code):
    """深いネストの output_dir でも正しく動作すること"""
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
            "output_dir": "deep/nested/output/"  # 深いネスト
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
    assert output_dir.exists(), "深いネストの output_dir が作成されていない"
    assert (output_dir / "config" / "functions.yml").exists()


def test_output_dir_dot_esb_relative(tmp_path, sample_template, sample_function_code):
    """output_dir が .esb/ (先頭ドット) で正しく動作すること"""
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
            "output_dir": ".esb/"  # 先頭ドット (hidden directory)
        }
    }
    
    # Execute
    functions = generator_main.generate_files(
        config=config,
        project_root=template_dir,
        dry_run=False,
        verbose=False
    )
    
    # Assert: テンプレートディレクトリ配下に作成されること
    output_dir = template_dir / ".esb"
    assert output_dir.exists(), ".esb/ がテンプレート相対で解決されていない"
    assert (output_dir / "config" / "functions.yml").exists()


def test_functions_yml_routing_yml_auto_derived(tmp_path, sample_template, sample_function_code):
    """functions_yml と routing_yml が指定されていない場合、output_dir/config/ に自動生成されること"""
    # Setup
    template_dir = tmp_path / "project"
    template_dir.mkdir()
    
    template_file = template_dir / "template.yaml"
    template_file.write_text(sample_template)
    
    src_dir = template_dir / "src"
    src_dir.mkdir()
    (src_dir / "app.py").write_text(sample_function_code)
    
    # functions_yml, routing_yml を明示的に指定しない
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
    
    # Assert: 自動的に output_dir/config/ に生成されること
    functions_yml = template_dir / "custom_output" / "config" / "functions.yml"
    routing_yml = template_dir / "custom_output" / "config" / "routing.yml"
    
    assert functions_yml.exists(), "functions.yml が自動生成されていない"
    assert routing_yml.exists(), "routing.yml が自動生成されていない"
    
    # 内容を確認
    with open(functions_yml) as f:
        content = yaml.safe_load(f)
    assert "functions" in content, "functions.yml に functions キーがない"
    assert "test-function" in content["functions"], "test-function が functions.yml に含まれていない"
