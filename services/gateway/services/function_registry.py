"""
Lambda関数レジストリ

functions.yml を読み込み、関数名→設定のマッピングを提供します。
デフォルト環境変数を関数固有の設定にマージします。
"""

from typing import Dict, Any, Optional
import yaml
import logging

from ..config import config

logger = logging.getLogger("gateway.function_registry")

# キャッシュされた関数設定
_function_registry: Dict[str, Dict[str, Any]] = {}
_defaults: Dict[str, Any] = {}


def load_functions_config() -> Dict[str, Dict[str, Any]]:
    """
    functions.yml を読み込んでキャッシュ

    Returns:
        関数名→設定の辞書
    """
    global _function_registry, _defaults

    try:
        with open(config.FUNCTIONS_CONFIG_PATH, "r", encoding="utf-8") as f:
            cfg = yaml.safe_load(f) or {}

        _defaults = cfg.get("defaults", {})
        _function_registry = cfg.get("functions", {})

        logger.info(
            f"Loaded {len(_function_registry)} functions from {config.FUNCTIONS_CONFIG_PATH}"
        )

    except FileNotFoundError:
        logger.warning(f"Functions config not found at {config.FUNCTIONS_CONFIG_PATH}")
        _function_registry = {}
        _defaults = {}

    except yaml.YAMLError as e:
        logger.error(f"Error parsing functions config: {e}")
        _function_registry = {}
        _defaults = {}

    return _function_registry


def get_function_config(function_name: str) -> Optional[Dict[str, Any]]:
    """
    関数名から設定を取得

    デフォルト環境変数を関数固有の設定にマージして返します。

    Args:
        function_name: 関数名（コンテナ名）

    Returns:
        関数設定（デフォルトマージ済み）。存在しない場合は None
    """
    if function_name not in _function_registry:
        return None

    func_config = _function_registry[function_name] or {}

    # デフォルト環境変数と関数固有の環境変数をマージ
    merged_env = {}
    default_env = _defaults.get("environment", {})
    func_env = func_config.get("environment", {})

    # デフォルト → 関数固有の順でマージ（関数固有が優先）
    merged_env.update(default_env)
    merged_env.update(func_env)

    # 結果を構築
    result = dict(func_config)
    result["environment"] = merged_env

    return result
