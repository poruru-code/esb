"""
Dockerfile and functions.yml Renderer

Generate Dockerfile and functions.yml from function info extracted from a SAM template.
"""

from pathlib import Path
from jinja2 import Environment, FileSystemLoader

TEMPLATE_DIR = Path(__file__).parent / "templates"


def render_dockerfile(
    func_config: dict,
    docker_config: dict,
    registry: str | None = None,
    tag: str = "latest",
) -> str:
    """
    Render a Dockerfile.

    Args:
        func_config: function settings
        docker_config: Docker settings
    """
    env = Environment(
        loader=FileSystemLoader(TEMPLATE_DIR),
        trim_blocks=True,
        lstrip_blocks=True,
    )
    template = env.get_template("Dockerfile.j2")

    # Extract Python version from runtime.
    runtime = func_config.get("runtime", "python3.12")
    python_version = runtime.replace("python", "")

    # Separate layers (zip vs directory).
    layers = func_config.get("layers", [])

    # Use provided registry address (External Address for build phase)
    if registry:
        base_image = f"{registry}/esb-lambda-base:{tag}"
    else:
        base_image = f"esb-lambda-base:{tag}"

    context = {
        "name": func_config.get("name", "unknown"),
        "python_version": python_version,
        "base_image": base_image,
        "sitecustomize_source": docker_config.get(
            "sitecustomize_source", "runtime/sitecustomize.py"
        ),
        "code_uri": func_config.get("code_uri", "./"),
        "handler": func_config.get("handler", "lambda_function.lambda_handler"),
        "has_requirements": func_config.get("has_requirements", False),
        "layers": layers,
    }

    return template.render(context)


def render_functions_yml(
    functions: list[dict],
    registry: str | None = None,
    tag: str = "latest",
) -> str:
    """
    Render functions.yml.

    Args:
        functions: list of functions
            - name: function name
            - environment: environment variable dict

    Returns:
        functions.yml string
    """
    env = Environment(
        loader=FileSystemLoader(TEMPLATE_DIR),
        trim_blocks=True,
        lstrip_blocks=True,
    )
    template = env.get_template("functions.yml.j2")

    return template.render(functions=functions, registry=registry, tag=tag)


def render_routing_yml(
    functions: list[dict],
) -> str:
    """
    Render routing.yml (Phase 1 extension).

    Args:
        functions: list of functions
            - name: function name
            - events: list of events (includes path, method)

    Returns:
        routing.yml string
    """
    env = Environment(
        loader=FileSystemLoader(TEMPLATE_DIR),
        trim_blocks=True,
        lstrip_blocks=True,
    )
    template = env.get_template("routing.yml.j2")

    return template.render(functions=functions)
