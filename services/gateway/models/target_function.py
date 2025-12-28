"""
TargetFunction model.

Data class representing the result of routing resolution.
"""

from typing import Dict, Any
from pydantic import BaseModel


class TargetFunction(BaseModel):
    """
    Lambda function information resolved by routing.
    """

    container_name: str
    path_params: Dict[str, str]
    route_path: str
    function_config: Dict[str, Any]
