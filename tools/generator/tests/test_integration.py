import tempfile
from pathlib import Path
from tools.generator.main import generate_files


class TestGeneratorIntegration:
    """Generator integration tests."""

    def test_generate_from_sam_template(self):
        """Ensure files can be generated from a SAM template."""
        sam_content = """
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31

Globals:
  Function:
    Runtime: python3.12
    Handler: lambda_function.lambda_handler

Resources:
  HelloFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-hello
      CodeUri: functions/hello/
      Events:
        ApiEvent:
          Type: Api
          Properties:
            Path: /api/hello
            Method: post
      ReservedConcurrentExecutions: 5
"""

        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)

            # Create a SAM template.
            sam_path = tmpdir / "template.yaml"
            sam_path.write_text(sam_content, encoding="utf-8")

            # Create function directory (source dir).
            src_dir = tmpdir / "src"
            func_src_dir = src_dir / "hello"
            func_src_dir.mkdir(parents=True)
            (func_src_dir / "lambda_function.py").write_text(
                "def lambda_handler(event, context): pass", encoding="utf-8"
            )

            # Create sitecustomize.py (tool resource) at the path main.py expects.
            # Default is: tools/generator/lib/sitecustomize.py
            # But the generator is running from tmpdir? No, main.py adds default string.
            # But renderer context uses that string in COPY.
            # Dockerfile generation does NOT check file existence on host.
            # So we don't strictly need to create it for GENERATION test.

            # Simple config.
            config = {
                "paths": {
                    "sam_template": str(sam_path),
                    "output_dir": str(tmpdir / "out"),
                    "functions_yml": str(tmpdir / "functions.yml"),
                    "routing_yml": str(tmpdir / "routing.yml"),
                },
                # docker config omitted to test default behavior
            }

            # Generate.
            # NOTE: renderer.py loads templates from file relative to renderer.py location.
            # This works even if processing in tmpdir as long as installed package is utilized.
            generate_files(config, project_root=tmpdir)

            # Verify.
            # Note: main.py uses output_dir / "functions" / func_name / "Dockerfile"
            dockerfile = tmpdir / "out" / "functions" / "lambda-hello" / "Dockerfile"
            assert dockerfile.exists(), f"Dockerfile should be generated at {dockerfile}"

            # content = dockerfile.read_text(encoding="utf-8")
            # assert "COPY tools/generator/runtime/sitecustomize.py" in content # Moved to base image

            functions_yml = tmpdir / "functions.yml"
            assert functions_yml.exists(), "functions.yml should be generated"

            routing_yml = tmpdir / "routing.yml"
            assert routing_yml.exists(), "routing.yml should be generated"

            routing_content = routing_yml.read_text(encoding="utf-8")
            assert "/api/hello" in routing_content
            assert "POST" in routing_content
            assert "lambda-hello" in routing_content

            functions_content = functions_yml.read_text(encoding="utf-8")
            assert "scaling:" in functions_content
            assert "max_capacity: 5" in functions_content
            assert "idle_timeout: 300" in functions_content
