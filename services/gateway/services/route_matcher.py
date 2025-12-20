"""
ルートマッチングサービス

routing.ymlを読み込み、リクエストパス/メソッドからターゲットコンテナを特定します。

Note:
    FastAPIの APIRouter とは異なる機能を提供します。
    このモジュールは設定ファイルベースのルーティングマッチングロジックです。
"""

import re
from typing import Optional, Tuple, Dict, Any, List
import yaml

from ..config import config

ROUTING_CONFIG_PATH = config.ROUTING_CONFIG_PATH

# キャッシュされたルーティング設定
_routing_config: List[Dict[str, Any]] = []


def load_routing_config() -> List[Dict[str, Any]]:
    """
    routing.ymlを読み込んでキャッシュ

    起動時に一度だけ呼び出される
    """
    global _routing_config

    try:
        with open(ROUTING_CONFIG_PATH, "r", encoding="utf-8") as f:
            cfg = yaml.safe_load(f)
            _routing_config = cfg.get("routes", [])
            print(f"Loaded {len(_routing_config)} routes from {ROUTING_CONFIG_PATH}")
    except FileNotFoundError:
        print(f"Warning: Routing config not found at {ROUTING_CONFIG_PATH}")
        _routing_config = []
    except yaml.YAMLError as e:
        print(f"Error parsing routing config: {e}")
        _routing_config = []

    return _routing_config


def get_routing_config() -> List[Dict[str, Any]]:
    """
    キャッシュされたルーティング設定を取得
    """
    if not _routing_config:
        load_routing_config()
    return _routing_config


def _path_to_regex(path_pattern: str) -> str:
    """
    パスパターンを正規表現に変換

    例: "/users/{user_id}/posts/{post_id}"
        → "^/users/(?P<user_id>[^/]+)/posts/(?P<post_id>[^/]+)$"
    """
    # {param} を名前付きキャプチャグループに置換
    regex_pattern = re.sub(r"\{(\w+)\}", r"(?P<\1>[^/]+)", path_pattern)
    return f"^{regex_pattern}$"


def match_route(
    request_path: str, request_method: str
) -> Tuple[Optional[str], Dict[str, str], Optional[str], Dict[str, Any]]:
    """
    リクエストパスとメソッドからターゲットコンテナを特定

    Args:
        request_path: リクエストパス (例: "/api/users/123")
        request_method: HTTPメソッド (例: "POST")

    Returns:
        Tuple of:
            - target_container: コンテナ名 (見つからない場合はNone)
            - path_params: パスパラメータの辞書
            - route_path: マッチしたルートのパスパターン (resource用)
            - function_config: function設定（image, environment等）
    """
    from .function_registry import get_function_config

    routes = get_routing_config()

    for route in routes:
        route_path = route.get("path", "")
        route_method = route.get("method", "").upper()

        # メソッドが一致するか確認
        if request_method.upper() != route_method:
            continue

        # パスパターンを正規表現に変換してマッチング
        regex_pattern = _path_to_regex(route_path)
        match = re.match(regex_pattern, request_path)

        if match:
            # パスパラメータを抽出
            path_params = match.groupdict()

            # function 設定を取得（新形式: 文字列、旧形式: 辞書）
            function_ref = route.get("function", {})

            if isinstance(function_ref, str):
                # 新形式: function_registry から設定を取得
                target_container = function_ref
                function_config = get_function_config(function_ref) or {}
            else:
                # 旧形式（後方互換）: 辞書から直接取得
                target_container = function_ref.get("container", "")
                function_config = function_ref

            return target_container, path_params, route_path, function_config

    # マッチするルートが見つからない
    return None, {}, None, {}


def extract_path_params(request_path: str, route_pattern: str) -> Dict[str, str]:
    """
    リクエストパスからパスパラメータを抽出

    Args:
        request_path: 実際のリクエストパス
        route_pattern: ルートパターン ({param}形式)

    Returns:
        パスパラメータの辞書
    """
    regex_pattern = _path_to_regex(route_pattern)
    match = re.match(regex_pattern, request_path)

    if match:
        return match.groupdict()
    return {}
