"""
Function domain models.

Defines the structure of a Lambda function configuration as a Pydantic model.
"""

from typing import Dict, List, Optional

from pydantic import BaseModel, Field


class ScalingConfig(BaseModel):
    """Configuration for auto-scaling and pool management."""

    min_capacity: int = 0
    max_capacity: int = 1
    idle_timeout: int = 300
    acquire_timeout: float = 30.0


class ScheduleEvent(BaseModel):
    """Lambda schedule event (Cron/Rate)."""

    rate: str
    input: Optional[str] = None


class FunctionEvent(BaseModel):
    """Generic event wrapper."""

    schedule: Optional[ScheduleEvent] = None


class FunctionEntity(BaseModel):
    """
    Core domain entity for a Lambda function.

    Represents the unified configuration after defaults are merged.
    """

    name: str
    timeout: int = 300
    memory_size: Optional[int] = None
    image: Optional[str] = None
    environment: Dict[str, str] = Field(default_factory=dict)
    scaling: ScalingConfig = Field(default_factory=ScalingConfig)
    events: List[FunctionEvent] = Field(default_factory=list)

    @classmethod
    def from_dict(cls, name: str, data: Dict) -> "FunctionEntity":
        """Factory to create from registry dict."""
        # Ensure scaling is a ScalingConfig object if it's a dict
        scaling_data = data.get("scaling")
        if isinstance(scaling_data, dict):
            scaling = ScalingConfig(**scaling_data)
        elif scaling_data is None:
            scaling = ScalingConfig()
        else:
            scaling = scaling_data

        return cls(
            name=name,
            timeout=data.get("timeout", 300),
            memory_size=data.get("memory_size"),
            image=data.get("image"),
            environment=data.get("environment", {}),
            scaling=scaling,
            events=data.get("events", []),
        )
