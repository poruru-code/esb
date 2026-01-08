import tempfile
import unittest
import zipfile
from pathlib import Path

from tools.generator.parser import parse_sam_template
from tools.generator.renderer import render_dockerfile


class TestLayerSupport(unittest.TestCase):
    def test_parse_layers(self):
        """Ensure AWS::Serverless::LayerVersion can be parsed."""
        template = """
        Resources:
          MyLayer:
            Type: AWS::Serverless::LayerVersion
            Properties:
              LayerName: my-common-layer
              ContentUri: layers/common/
              CompatibleRuntimes:
                - python3.12
          
          MyFunction:
            Type: AWS::Serverless::Function
            Properties:
              CodeUri: functions/my-func/
              Layers:
                - !Ref MyLayer
        """

        parsed = parse_sam_template(template)

        # Ensure layer info is attached to the function.
        functions = parsed["functions"]
        self.assertEqual(len(functions), 1)
        func = functions[0]
        self.assertIn("layers", func)
        self.assertEqual(len(func["layers"]), 1)

        layer = func["layers"][0]
        self.assertEqual(layer["name"], "my-common-layer")
        self.assertEqual(layer["content_uri"], "layers/common/")

    def test_render_dockerfile_with_layers(self):
        """Ensure a Dockerfile with layers is generated correctly."""
        func_config = {
            "name": "lambda-my-func",
            "runtime": "python3.12",
            "layers": [{"name": "my-common-layer", "content_uri": "layers/common"}],
        }

        docker_config = {}

        dockerfile = render_dockerfile(func_config, docker_config)

        # Ensure COPY commands are present.
        self.assertIn("COPY layers/common/ /opt/", dockerfile)

    def test_parse_layers_from_globals(self):
        """Ensure layers defined in Globals are applied to functions."""
        template = """
        Globals:
          Function:
            Layers:
              - !Ref CommonLayer

        Resources:
          CommonLayer:
            Type: AWS::Serverless::LayerVersion
            Properties:
              LayerName: common-layer
              ContentUri: layers/common/

          MyFunction:
            Type: AWS::Serverless::Function
            Properties:
              CodeUri: functions/my-func/
        """
        parsed = parse_sam_template(template)
        functions = parsed["functions"]
        self.assertEqual(len(functions), 1)
        func = functions[0]

        # Ensure the Globals layer is applied.
        self.assertIn("layers", func)
        self.assertEqual(len(func["layers"]), 1)
        self.assertEqual(func["layers"][0]["name"], "common-layer")
    def test_render_dockerfile_with_zip_layer(self):
        """Ensure staged zip layers generate the expected Dockerfile output."""
        from tools.generator import renderer
        func = {
            "name": "test-func",
            "code_uri": "./app/",
            "handler": "app.handler",
            "runtime": "python3.12",
            "layers": [
                {"name": "lib-layer", "content_uri": "layers/lib"},
                {"name": "common-layer", "content_uri": "layers/common"}
            ]
        }
        docker_config = {}
        
        output = renderer.render_dockerfile(func, docker_config)
        
        # Renderer now just iterates and copies the staged layer directories.
        self.assertIn("COPY layers/lib/ /opt/", output)
        self.assertIn("COPY layers/common/ /opt/", output)

    def test_generate_files_dedupes_layers_and_unzips(self):
        """Ensure shared layers are staged once and zip layers are unpacked."""
        from tools.generator.main import generate_files

        template = """
        AWSTemplateFormatVersion: "2010-09-09"
        Transform: AWS::Serverless-2016-10-31
        Resources:
          CommonLayer:
            Type: AWS::Serverless::LayerVersion
            Properties:
              LayerName: common-layer
              ContentUri: layers/common/
          ZipLayer:
            Type: AWS::Serverless::LayerVersion
            Properties:
              LayerName: zip-layer
              ContentUri: layers/zip-layer.zip
          FuncOne:
            Type: AWS::Serverless::Function
            Properties:
              FunctionName: lambda-one
              CodeUri: functions/one/
              Layers:
                - !Ref CommonLayer
                - !Ref ZipLayer
          FuncTwo:
            Type: AWS::Serverless::Function
            Properties:
              FunctionName: lambda-two
              CodeUri: functions/two/
              Layers:
                - !Ref CommonLayer
                - !Ref ZipLayer
        """

        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)

            # Create layer directory content.
            common_layer_dir = tmpdir / "layers" / "common" / "python" / "common"
            common_layer_dir.mkdir(parents=True, exist_ok=True)
            (common_layer_dir / "__init__.py").write_text("# common layer", encoding="utf-8")

            # Create zip layer content.
            zip_path = tmpdir / "layers" / "zip-layer.zip"
            zip_path.parent.mkdir(parents=True, exist_ok=True)
            with zipfile.ZipFile(zip_path, "w") as zip_ref:
                zip_ref.writestr("python/zip_layer/__init__.py", "# zip layer")

            # Create function source directories.
            for name in ["one", "two"]:
                func_dir = tmpdir / "functions" / name
                func_dir.mkdir(parents=True, exist_ok=True)
                (func_dir / "lambda_function.py").write_text(
                    "def lambda_handler(event, context): pass", encoding="utf-8"
                )

            # Write SAM template.
            template_path = tmpdir / "template.yaml"
            template_path.write_text(template, encoding="utf-8")

            config = {
                "paths": {
                    "sam_template": str(template_path),
                    "output_dir": str(tmpdir / "out"),
                }
            }

            generate_files(config, project_root=tmpdir, dry_run=False, verbose=False)

            out_dir = tmpdir / "out"
            layers_dir = out_dir / "layers"
            self.assertTrue(layers_dir.exists())

            layer_dirs = [p for p in layers_dir.iterdir() if p.is_dir()]
            self.assertEqual(len(layer_dirs), 2)

            zip_unpacked = any(
                (layer_dir / "python" / "zip_layer" / "__init__.py").exists()
                for layer_dir in layer_dirs
            )
            self.assertTrue(zip_unpacked)

            for func_name in ["lambda-one", "lambda-two"]:
                func_layers_dir = out_dir / "functions" / func_name / "layers"
                self.assertFalse(func_layers_dir.exists())

                dockerfile = (out_dir / "functions" / func_name / "Dockerfile").read_text(
                    encoding="utf-8"
                )
                for layer_dir in layer_dirs:
                    self.assertIn(
                        f"COPY layers/{layer_dir.name}/ /opt/",
                        dockerfile,
                    )
