"""
Invocation result models.

Standardizes the output of the Lambda invocation pipeline.
"""

from typing import Dict, List, Optional

from pydantic import BaseModel, Field


class InvocationResult(BaseModel):
    """
    Unified result of a Lambda invocation.

    Used to decouple the internal pipeline from FastAPI Response objects.
    """

    success: bool
    status_code: int
    payload: bytes = b""
    headers: Dict[str, str] = Field(default_factory=dict)
    multi_headers: Dict[str, List[str]] = Field(default_factory=dict)
    error: Optional[str] = None
    is_retryable: bool = False

    @property
    def is_logic_error(self) -> bool:
        """Returns True if it's a Lambda logical error (X-Amz-Function-Error)."""
        return self.headers.get("X-Amz-Function-Error") is not None
