from pathlib import Path


import os


def find_project_root(current_path: Path = None) -> Path:
    """pyproject.toml を探してプロジェクトルートを特定する"""
    if current_path is None:
        current_path = Path.cwd()

    for path in [current_path] + list(current_path.parents):
        if (path / "pyproject.toml").exists():
            return path

    return Path(__file__).parent.parent.parent.resolve()


PROJECT_ROOT = find_project_root()
TOOLS_DIR = PROJECT_ROOT / "tools"
GENERATOR_DIR = TOOLS_DIR / "generator"
PROVISIONER_DIR = TOOLS_DIR / "provisioner"

# パス優先順位:
# 1. 環境変数 ESB_TEMPLATE
# 2. カレントディレクトリの template.yaml
# 3. プロジェクトルート直下の template.yaml
# 4. tests/fixtures/template.yaml (デフォルト)

env_template = os.environ.get("ESB_TEMPLATE")
if env_template:
    TEMPLATE_YAML = Path(env_template).resolve()
elif (Path.cwd() / "template.yaml").exists():
    TEMPLATE_YAML = Path.cwd() / "template.yaml"
elif (PROJECT_ROOT / "template.yaml").exists():
    TEMPLATE_YAML = PROJECT_ROOT / "template.yaml"
else:
    TEMPLATE_YAML = PROJECT_ROOT / "tests" / "e2e" / "template.yaml"

# テンプレートがあるディレクトリを E2E_DIR 相当として扱う (Dockerfile等の探索起点)
E2E_DIR = TEMPLATE_YAML.parent

# 出力パス設定
DEFAULT_ROUTING_YML = E2E_DIR / "config" / "routing.yml"
DEFAULT_FUNCTIONS_YML = E2E_DIR / "config" / "functions.yml"
