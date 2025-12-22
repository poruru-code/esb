"""
Dockerfile and functions.yml Renderer

SAMテンプレートから抽出した関数情報をもとに、
DockerfileとFunctions.ymlを生成します。
"""

from pathlib import Path
from jinja2 import Environment, FileSystemLoader

TEMPLATE_DIR = Path(__file__).parent / "templates"


def render_dockerfile(
    func_config: dict,
    docker_config: dict,
) -> str:
    """
    Dockerfileをレンダリングする

    Args:
        func_config: 関数設定
            - name: 関数名
            - code_uri: コードパス
            - handler: ハンドラ
            - runtime: ランタイム (e.g., 'python3.12')
            - has_requirements: requirements.txt があるか
        docker_config: Docker設定
            - sitecustomize_source: sitecustomize.pyのパス

    Returns:
        Dockerfile文字列
    """
    env = Environment(loader=FileSystemLoader(TEMPLATE_DIR))
    template = env.get_template("Dockerfile.j2")

    # ランタイムからPythonバージョンを抽出 (e.g., 'python3.12' -> '3.12')
    runtime = func_config.get("runtime", "python3.12")
    python_version = runtime.replace("python", "")

    context = {
        "python_version": python_version,
        "sitecustomize_source": docker_config.get(
            "sitecustomize_source", "runtime/sitecustomize.py"
        ),
        "code_uri": func_config.get("code_uri", "./"),
        "handler": func_config.get("handler", "lambda_function.lambda_handler"),
        "has_requirements": func_config.get("has_requirements", False),
    }

    return template.render(context)


def render_functions_yml(
    functions: list[dict],
) -> str:
    """
    functions.yml をレンダリングする

    Args:
        functions: 関数リスト
            - name: 関数名
            - environment: 環境変数辞書

    Returns:
        functions.yml文字列
    """
    env = Environment(loader=FileSystemLoader(TEMPLATE_DIR))
    template = env.get_template("functions.yml.j2")

    return template.render(functions=functions)
