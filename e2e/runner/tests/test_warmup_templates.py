# Where: e2e/runner/tests/test_warmup_templates.py
# What: Unit tests for template runtime detection used by warmup logic.
# Why: Prevent regressions in Java fixture warmup eligibility checks.
from __future__ import annotations

import textwrap
from pathlib import Path

from e2e.runner.warmup import _template_has_java_runtime


def _write_template(tmp_path: Path, body: str) -> Path:
    path = tmp_path / "template.yaml"
    path.write_text(textwrap.dedent(body).strip() + "\n", encoding="utf-8")
    return path


def test_template_has_java_runtime_from_globals(tmp_path):
    template = _write_template(
        tmp_path,
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
    assert _template_has_java_runtime(template)


def test_template_has_no_java_runtime(tmp_path):
    template = _write_template(
        tmp_path,
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
    assert not _template_has_java_runtime(template)


def test_template_with_cfn_tag_still_detects_java(tmp_path):
    template = _write_template(
        tmp_path,
        """
        AWSTemplateFormatVersion: "2010-09-09"
        Transform: AWS::Serverless-2016-10-31
        Resources:
          CommonLayer:
            Type: AWS::Serverless::LayerVersion
            Properties:
              LayerName: common-lib
              ContentUri: layers/common/
          JavaEchoFunction:
            Type: AWS::Serverless::Function
            Properties:
              FunctionName: lambda-echo-java
              Runtime: java21
              Layers:
                - !Ref CommonLayer
              CodeUri: functions/java/echo/app.jar
        """,
    )
    assert _template_has_java_runtime(template)
