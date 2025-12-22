from pathlib import Path


def find_project_root(current_path: Path = None) -> Path:
    """pyproject.toml を探してプロジェクトルートを特定する"""
    if current_path is None:
        current_path = Path.cwd()

    for path in [current_path] + list(current_path.parents):
        if (path / "pyproject.toml").exists():
            return path

    # 見つからない場合はスクリプトの場所から推測
    return Path(__file__).parent.parent.parent.resolve()


PROJECT_ROOT = find_project_root()
TOOLS_DIR = PROJECT_ROOT / "tools"
GENERATOR_DIR = TOOLS_DIR / "generator"
PROVISIONER_DIR = TOOLS_DIR / "provisioner"
TESTS_DIR = PROJECT_ROOT / "tests"
E2E_DIR = TESTS_DIR / "e2e"
TEMPLATE_YAML = E2E_DIR / "template.yaml"

# 出力パス設定
DEFAULT_ROUTING_YML = E2E_DIR / "config" / "routing.yml"
DEFAULT_FUNCTIONS_YML = E2E_DIR / "config" / "functions.yml"
