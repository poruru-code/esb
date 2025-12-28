from typing import Optional, Dict, List
from dataclasses import dataclass
from pydantic import BaseModel, Field


# =============================================================================
# Auto-Scaling Data Structures
# =============================================================================


@dataclass
class WorkerInfo:
    """
    Metadata required for container state management.

    Auto-scaling support:
    - Set frozen=False (to update last_used_at)
    - Use id-based __eq__/__hash__ (identity in Set/Dict)
    """

    id: str  # Container ID (Docker ID)
    name: str  # Container name (lambda-{function}-{suffix})
    ip_address: str  # Container IP (for execution)
    port: int = 8080  # Service port
    created_at: float = 0.0  # Creation time
    last_used_at: float = 0.0  # Last used time (for auto-scaling)

    def __eq__(self, other):
        if isinstance(other, WorkerInfo):
            return self.id == other.id
        return False

    def __hash__(self):
        return hash(self.id)


class ContainerProvisionRequest(BaseModel):
    """Gateway -> Manager: container provisioning request."""

    function_name: str = Field(..., description="Function name")
    count: int = Field(default=1, ge=1, le=10, description="Number of containers to create")
    image: Optional[str] = Field(None, description="Docker image to use")
    env: Dict[str, str] = Field(default_factory=dict, description="Environment variables to inject")
    request_id: Optional[str] = Field(None, description="Request ID for tracing")
    dry_run: bool = Field(default=False, description="Dry run")


class ContainerProvisionResponse(BaseModel):
    """Manager -> Gateway: provisioning result."""

    workers: List[WorkerInfo] = Field(..., description="List of created workers")


class HeartbeatRequest(BaseModel):
    """Gateway -> Manager: heartbeat (for Janitor)."""

    function_name: str = Field(..., description="Function name")
    container_names: List[str] = Field(
        ..., description="List of container names currently in the pool"
    )


# =============================================================================
# Existing Models (Legacy - ensure API)
# =============================================================================


class ContainerEnsureRequest(BaseModel):
    """
    Gateway -> Manager: container start request.
    """

    function_name: str = Field(..., description="Target function name (container name)")
    image: Optional[str] = Field(None, description="Docker image to use")
    env: Dict[str, str] = Field(default_factory=dict, description="Environment variables to inject")


class ContainerInfoResponse(BaseModel):
    """
    Manager -> Gateway: container connection information.
    """

    host: str = Field(..., description="Container host name or IP")
    port: int = Field(..., description="Service port")
