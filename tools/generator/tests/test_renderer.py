from tools.generator.renderer import render_dockerfile, render_functions_yml


class TestDockerfileRenderer:
    """Dockerfileレンダラーのテスト"""

    def test_render_simple_dockerfile(self):
        """シンプルなDockerfileを生成できる"""
        func_config = {
            "name": "lambda-hello",
            "code_uri": "functions/hello/",
            "handler": "lambda_function.lambda_handler",
            "runtime": "python3.12",
            "environment": {},
        }

        docker_config = {
            "sitecustomize_source": "tools/generator/runtime/sitecustomize.py",
        }

        result = render_dockerfile(func_config, docker_config)

        assert "FROM public.ecr.aws/lambda/python:3.12" in result
        assert "COPY tools/generator/runtime/sitecustomize.py" in result
        assert "COPY functions/hello/" in result
        assert 'CMD [ "lambda_function.lambda_handler" ]' in result

    def test_render_dockerfile_with_requirements(self):
        """requirements.txt がある場合 pip install を含む"""
        func_config = {
            "name": "lambda-hello",
            "code_uri": "functions/hello/",
            "handler": "lambda_function.lambda_handler",
            "runtime": "python3.12",
            "environment": {},
            "has_requirements": True,
        }

        docker_config = {
            "sitecustomize_source": "tools/generator/runtime/sitecustomize.py",
        }

        result = render_dockerfile(func_config, docker_config)

        assert "pip install -r" in result


class TestFunctionsYmlRenderer:
    """functions.yml レンダラーのテスト"""

    def test_render_functions_yml(self):
        """functions.yml を生成できる"""
        functions = [
            {
                "name": "lambda-hello",
                "environment": {},
            },
            {
                "name": "lambda-s3-test",
                "environment": {
                    "S3_ENDPOINT": "http://onpre-storage:9000",
                },
            },
        ]

        # NOTE: base_config argument is removed as defaults are now embedded/templatized
        result = render_functions_yml(functions)

        assert "defaults:" in result
        # Check defaults embedded in template
        assert "GATEWAY_INTERNAL_URL" in result
        assert "lambda-hello" in result
        assert "lambda-s3-test" in result
        assert "S3_ENDPOINT" in result

    def test_render_routing_yml(self):
        """routing.yml を生成できる"""
        from tools.generator.renderer import render_routing_yml

        functions = [
            {
                "name": "lambda-hello",
                "events": [{"path": "/api/hello", "method": "post"}],
            },
            {
                "name": "lambda-s3-test",
                "events": [
                    {"path": "/api/s3/test", "method": "post"},
                    {"path": "/api/s3/check", "method": "get"},
                ],
            },
        ]

        result = render_routing_yml(functions)

        assert "/api/hello" in result
        assert "POST" in result
        assert "lambda-hello" in result
        assert "/api/s3/test" in result
        assert "/api/s3/check" in result
        assert "GET" in result
