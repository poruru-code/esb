# Where: e2e/runner/test_runner_java_warmup.py
# What: Unit tests for Java warmup template detection.
# Why: Ensure Java fixtures warm up when templates declare Java runtimes.
import textwrap
from pathlib import Path

from e2e.runner import runner


def _write(tmp_path: Path, name: str, content: str) -> Path:
    path = tmp_path / name
    path.write_text(textwrap.dedent(content).strip() + "\n", encoding="utf-8")
    return path


def test_template_has_java_runtime_from_globals(tmp_path):
    template = _write(
        tmp_path,
        "template.yaml",
        """
        AWSTemplateFormatVersion: "2010-09-09"
        Transform: AWS::Serverless-2016-10-31
        Globals:
          Function:
            Runtime: java21
        Resources:
          EchoFunction:
            Type: AWS::Serverless::Function
            Properties:
              FunctionName: lambda-echo
              CodeUri: functions/python/echo/
        """,
    )
    assert runner._template_has_java_runtime(template)


def test_template_has_java_runtime_from_resource_runtime(tmp_path):
    template = _write(
        tmp_path,
        "template.yaml",
        """
        AWSTemplateFormatVersion: "2010-09-09"
        Transform: AWS::Serverless-2016-10-31
        Resources:
          JavaEchoFunction:
            Type: AWS::Serverless::Function
            Properties:
              FunctionName: lambda-echo-java
              Runtime: java21
              CodeUri: functions/java/echo/app.jar
        """,
    )
    assert runner._template_has_java_runtime(template)


def test_template_has_java_runtime_with_cfn_tags(tmp_path):
    template = _write(
        tmp_path,
        "template.yaml",
        """
        AWSTemplateFormatVersion: "2010-09-09"
        Transform: AWS::Serverless-2016-10-31
        Globals:
          Function:
            Layers:
              - !Ref CommonLayer
        Resources:
          CommonLayer:
            Type: AWS::Serverless::LayerVersion
            Properties:
              LayerName: common-lib
              ContentUri: layers/common/
              CompatibleRuntimes:
                - python3.12
          JavaEchoFunction:
            Type: AWS::Serverless::Function
            Properties:
              FunctionName: lambda-echo-java
              Runtime: java21
              CodeUri: functions/java/echo/app.jar
        """,
    )
    assert runner._template_has_java_runtime(template)


def test_template_has_java_runtime_from_codeuri_only(tmp_path):
    template = _write(
        tmp_path,
        "template.yaml",
        """
        AWSTemplateFormatVersion: "2010-09-09"
        Transform: AWS::Serverless-2016-10-31
        Resources:
          JavaEchoFunction:
            Type: AWS::Serverless::Function
            Properties:
              FunctionName: lambda-echo-java
              CodeUri: functions/java/echo/app.jar
        """,
    )
    assert runner._template_has_java_runtime(template)


def test_template_has_no_java_runtime(tmp_path):
    template = _write(
        tmp_path,
        "template.yaml",
        """
        AWSTemplateFormatVersion: "2010-09-09"
        Transform: AWS::Serverless-2016-10-31
        Globals:
          Function:
            Runtime: python3.12
        Resources:
          EchoFunction:
            Type: AWS::Serverless::Function
            Properties:
              FunctionName: lambda-echo
              CodeUri: functions/python/echo/
        """,
    )
    assert not runner._template_has_java_runtime(template)
