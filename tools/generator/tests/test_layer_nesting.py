import unittest
import tempfile
import zipfile
import shutil
from pathlib import Path
from tools.generator.main import generate_files


class TestLayerNesting(unittest.TestCase):
    """Test the smart nesting logic for Python layers."""

    def test_layer_nesting_scenarios(self):
        """
        Verify that:
        1. Flat Directory Layer -> Nested into python/
        2. Nested Directory Layer (has python/) -> Kept as is
        3. Flat Zip Layer -> Nested into python/
        4. Nested Zip Layer (has python/) -> Kept as is
        """
        sam_template = """
        Resources:
          MyFunc:
            Type: AWS::Serverless::Function
            Properties:
              FunctionName: lambda-nesting-test
              Runtime: python3.12
              CodeUri: functions/my-func/
              Handler: app.handler
              Layers:
                - !Ref FlatDirLayer
                - !Ref NestedDirLayer
                - !Ref FlatZipLayer
                - !Ref NestedZipLayer

          FlatDirLayer:
            Type: AWS::Serverless::LayerVersion
            Properties:
              LayerName: layer-flat-dir
              ContentUri: layers/flat_dir
          
          NestedDirLayer:
            Type: AWS::Serverless::LayerVersion
            Properties:
              LayerName: layer-nested-dir
              ContentUri: layers/nested_dir

          FlatZipLayer:
            Type: AWS::Serverless::LayerVersion
            Properties:
              LayerName: layer-flat-zip
              ContentUri: layers/flat.zip

          NestedZipLayer:
            Type: AWS::Serverless::LayerVersion
            Properties:
              LayerName: layer-nested-zip
              ContentUri: layers/nested.zip
        """

        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)

            # Setup SAM Template
            (tmpdir / "template.yaml").write_text(sam_template, encoding="utf-8")

            # Setup Function Source
            (tmpdir / "functions" / "my-func").mkdir(parents=True)
            (tmpdir / "functions" / "my-func" / "app.py").touch()

            # 1. Flat Directory Layer (Should be nested)
            flat_dir = tmpdir / "layers" / "flat_dir"
            flat_dir.mkdir(parents=True)
            (flat_dir / "lib_flat.py").touch()

            # 2. Nested Directory Layer (Should NOT be nested)
            nested_dir = tmpdir / "layers" / "nested_dir"
            nested_dir.mkdir(parents=True)
            (nested_dir / "python").mkdir()
            (nested_dir / "python" / "lib_nested.py").touch()

            # 3. Flat Zip Layer (Should be nested)
            flat_zip = tmpdir / "layers" / "flat.zip"
            with zipfile.ZipFile(flat_zip, "w") as z:
                z.writestr("lib_zip_flat.py", "print('hello')")

            # 4. Nested Zip Layer (Should NOT be nested)
            nested_zip = tmpdir / "layers" / "nested.zip"
            with zipfile.ZipFile(nested_zip, "w") as z:
                z.writestr("python/lib_zip_nested.py", "print('hello')")

            # Run Generator
            config = {
                "paths": {
                    "sam_template": str(tmpdir / "template.yaml"),
                    "output_dir": str(tmpdir / "out"),
                }
            }
            # Run with verbose=False to keep test output clean
            generate_files(config, project_root=tmpdir, verbose=False)

            # Verify Integrity
            staging_root = tmpdir / "out" / "functions" / "lambda-nesting-test" / "layers"

            # Check Flat Dir -> Nested
            # Expected: layers/layer-flat-dir/python/lib_flat.py
            p1 = staging_root / "layer-flat-dir" / "python" / "lib_flat.py"
            self.assertTrue(p1.exists(), f"Flat Dir Layer should be nested. Missing: {p1}")

            # Check Nested Dir -> Not Cached (Preserves structure)
            # Expected: layers/layer-nested-dir/python/lib_nested.py
            p2 = staging_root / "layer-nested-dir" / "python" / "lib_nested.py"
            self.assertTrue(p2.exists(), f"Nested Dir Layer should exist. Missing: {p2}")
            # Ensure NO double nesting
            p2_double = staging_root / "layer-nested-dir" / "python" / "python"
            self.assertFalse(p2_double.exists(), "Nested Dir Layer should NOT be double nested")

            # Check Flat Zip -> Nested
            # Expected: layers/layer-flat-zip/python/lib_zip_flat.py
            p3 = staging_root / "layer-flat-zip" / "python" / "lib_zip_flat.py"
            self.assertTrue(p3.exists(), f"Flat Zip Layer should be nested. Missing: {p3}")

            # Check Nested Zip -> Not Nested
            # Expected: layers/layer-nested-zip/python/lib_zip_nested.py
            p4 = staging_root / "layer-nested-zip" / "python" / "lib_zip_nested.py"
            self.assertTrue(p4.exists(), f"Nested Zip Layer should exist. Missing: {p4}")
            # Ensure NO double nesting
            p4_double = staging_root / "layer-nested-zip" / "python" / "python"
            self.assertFalse(p4_double.exists(), "Nested Zip Layer should NOT be double nested")


if __name__ == "__main__":
    unittest.main()
