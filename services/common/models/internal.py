from typing import Optional, Dict
from pydantic import BaseModel, Field


class ContainerEnsureRequest(BaseModel):
    """
    Gateway -> Manager: コンテナ起動リクエスト
    """

    function_name: str = Field(..., description="起動対象の関数名（コンテナ名）")
    image: Optional[str] = Field(None, description="使用するDockerイメージ")
    env: Dict[str, str] = Field(default_factory=dict, description="注入する環境変数")


class ContainerInfoResponse(BaseModel):
    """
    Manager -> Gateway: コンテナ接続情報
    """

    host: str = Field(..., description="コンテナのホスト名またはIP")
    port: int = Field(..., description="サービスポート番号")
