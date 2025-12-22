import tempfile
from pathlib import Path
from tools.generator.main import generate_files


class TestGeneratorIntegration:
    """ジェネレータ統合テスト"""

    def test_generate_from_sam_template(self):
        """SAMテンプレートからファイルを生成できる"""
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
"""

        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)

            # SAMテンプレートを作成
            sam_path = tmpdir / "template.yaml"
            sam_path.write_text(sam_content, encoding="utf-8")

            # 関数ディレクトリを作成
            func_dir = tmpdir / "functions" / "hello"
            func_dir.mkdir(parents=True)
            (func_dir / "lambda_function.py").write_text(
                "def lambda_handler(event, context): pass", encoding="utf-8"
            )

            # sitecustomize.py (ツールリソース) を作成 (main.pyが探すパス)
            # Default is: tools/generator/lib/sitecustomize.py
            # But the generator is running from tmpdir? No, main.py adds default string.
            # But renderer context uses that string in COPY.
            # Dockerfile generation does NOT check file existence on host.
            # So we don't strictly need to create it for GENERATION test.

            # 簡易設定
            config = {
                "paths": {
                    "sam_template": str(sam_path),
                    "output_dir": str(tmpdir / "functions"),
                    "functions_yml": str(tmpdir / "functions.yml"),
                    "routing_yml": str(tmpdir / "routing.yml"),
                },
                # docker config omitted to test default behavior
            }

            # 生成実行
            # NOTE: renderer.py loads templates from file relative to renderer.py location.
            # This works even if processing in tmpdir as long as installed package is utilized.
            generate_files(config, project_root=tmpdir)

            # 検証
            dockerfile = func_dir / "Dockerfile"
            assert dockerfile.exists(), "Dockerfile should be generated"

            content = dockerfile.read_text(encoding="utf-8")
            assert "COPY tools/generator/runtime/sitecustomize.py" in content

            functions_yml = tmpdir / "functions.yml"
            assert functions_yml.exists(), "functions.yml should be generated"

            routing_yml = tmpdir / "routing.yml"
            assert routing_yml.exists(), "routing.yml should be generated"

            routing_content = routing_yml.read_text(encoding="utf-8")
            assert "/api/hello" in routing_content
            assert "POST" in routing_content
            assert "lambda-hello" in routing_content
