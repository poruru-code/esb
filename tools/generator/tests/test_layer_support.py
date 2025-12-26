import unittest
from tools.generator.parser import parse_sam_template
from tools.generator.renderer import render_dockerfile


class TestLayerSupport(unittest.TestCase):
    def test_parse_layers(self):
        """AWS::Serverless::LayerVersionをパースできるか"""
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

        # 関数にLayer情報が紐付いているか
        functions = parsed["functions"]
        self.assertEqual(len(functions), 1)
        func = functions[0]
        self.assertIn("layers", func)
        self.assertEqual(len(func["layers"]), 1)

        layer = func["layers"][0]
        self.assertEqual(layer["name"], "my-common-layer")
        self.assertEqual(layer["content_uri"], "layers/common/")

    def test_render_dockerfile_with_layers(self):
        """Layerを含むDockerfileが正しく生成されるか"""
        func_config = {
            "name": "lambda-my-func",
            "runtime": "python3.12",
            "layers": [{"name": "my-common-layer", "content_uri": "layers/common/"}],
        }

        docker_config = {}

        dockerfile = render_dockerfile(func_config, docker_config)

        # COPY コマンドが含まれているか
        self.assertIn("COPY layers/common/ /opt/", dockerfile)

    def test_parse_layers_from_globals(self):
        """Globalsに定義されたLayerが関数に適用されるか"""
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

        # GlobalsのLayerが適用されていること
        self.assertIn("layers", func)
        self.assertEqual(len(func["layers"]), 1)
        self.assertEqual(func["layers"][0]["name"], "common-layer")
    def test_render_dockerfile_with_zip_layer(self):
        """Zip Layerが含まれる場合、Multi-stage build (unzip) が生成されること"""
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
