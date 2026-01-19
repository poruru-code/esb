"""
Input context models.

Encapsulates all data required to process a gateway request.
"""

from typing import Dict, List, Optional

from pydantic import BaseModel, Field


class InputContext(BaseModel):
    """
    Rich context representing an incoming request.

    This model decouples the service layer from FastAPI's Request object.
    """

    function_name: str
    method: str
    path: str
    headers: Dict[str, str]
    multi_headers: Dict[str, List[str]] = Field(default_factory=dict)
    query_params: Dict[str, str] = Field(default_factory=dict)
    multi_query_params: Dict[str, List[str]] = Field(default_factory=dict)
    body: bytes = b""
    user_id: Optional[str] = None
    path_params: Dict[str, str] = Field(default_factory=dict)
    route_path: Optional[str] = None
    timeout: float = 30.0
