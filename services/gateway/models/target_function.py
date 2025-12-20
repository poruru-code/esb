"""
TargetFunction モデル

ルーティングの解決結果を表すデータクラス。
"""

from typing import Dict, Any
from pydantic import BaseModel


class TargetFunction(BaseModel):
    """
    ルーティング解決によって特定された Lambda 関数の情報
    """

    container_name: str
    path_params: Dict[str, str]
    route_path: str
    function_config: Dict[str, Any]
