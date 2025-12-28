import unittest
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
            "layers": [{"name": "my-common-layer", "content_uri": "layers/common/"}],
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
        """Ensure zip layers generate the expected Dockerfile output."""
        from tools.generator import renderer
        func = {
            "name": "test-func",
            "code_uri": "./app/",
            "handler": "app.handler",
            "runtime": "python3.12",
            "layers": [
                {"name": "lib-layer", "content_uri": "./layers/lib.zip"}, 
                {"name": "common-layer", "content_uri": "./layers/common/"}
            ]
        }
        docker_config = {}
        
        output = renderer.render_dockerfile(func, docker_config)
        
        # Renderer now just iterates and copies whatever URI is given
        self.assertIn("COPY ./layers/lib.zip /opt/", output)
        self.assertIn("COPY ./layers/common/ /opt/", output)
